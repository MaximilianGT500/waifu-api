package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngWithHeader(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8
	ihdr[9] = 2
	writePNGChunk(&buf, "IHDR", ihdr)
	writePNGChunk(&buf, "IDAT", []byte{0x78, 0x9c, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01})
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(data)))
	buf.Write(l[:])
	buf.WriteString(typ)
	buf.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], crc.Sum32())
	buf.Write(c[:])
}

func TestProbeRejectsDecompressionBombBeforeDecoding(t *testing.T) {
	data := pngWithHeader(50000, 50000)
	info, err := Probe(data)
	if err != nil {
		t.Fatalf("probe should succeed on header-only data: %v", err)
	}
	if info.Width != 50000 || info.Height != 50000 {
		t.Fatalf("unexpected dims %dx%d", info.Width, info.Height)
	}
	if err := ValidateDimensions(info, 1, 1, 9000, 9000, 1, 1, 2000, 2000); err == nil {
		t.Fatal("expected dimension validation to reject the bomb")
	}
	if _, err := Decode(data, info, 81_000_000); !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("expected ErrTooManyPixels, got %v", err)
	}
}

func TestProbeAndDecodePNG(t *testing.T) {
	data := encodePNG(t, 120, 80)
	info, err := Probe(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "png" || info.IsAnimated || info.Width != 120 || info.Height != 80 {
		t.Fatalf("unexpected info %+v", info)
	}
	img, err := Decode(data, info, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 120 {
		t.Fatalf("unexpected decoded width %d", img.Bounds().Dx())
	}
	if ContentType(info.Format) != "image/png" || Extension(info.Format, ".html") != ".png" {
		t.Fatal("content type / extension must come from the detected format")
	}
}

func TestExtensionKeepsValidJPEGSpelling(t *testing.T) {
	if Extension("jpeg", ".jpeg") != ".jpeg" || Extension("jpeg", ".jpg") != ".jpg" || Extension("jpeg", ".php") != ".jpg" {
		t.Fatal("unexpected jpeg extension handling")
	}
}

func TestProbeRejectsUnsupportedAndGarbage(t *testing.T) {
	if _, err := Probe([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt for svg, got %v", err)
	}
	bmp := []byte("BM" + string(make([]byte, 60)))
	binary.LittleEndian.PutUint32(bmp[10:], 54)
	binary.LittleEndian.PutUint32(bmp[14:], 40)
	binary.LittleEndian.PutUint32(bmp[18:], 2)
	binary.LittleEndian.PutUint32(bmp[22:], 2)
	binary.LittleEndian.PutUint16(bmp[26:], 1)
	binary.LittleEndian.PutUint16(bmp[28:], 24)
	if _, err := Probe(bmp); err == nil {
		t.Fatal("bmp must be rejected")
	}
}

func TestGIFDecodesOnlyFirstFrame(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	g := &gif.GIF{}
	for i := 0; i < 50; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 200, 150), pal)
		frame.SetColorIndex(i%200, 0, 1)
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 1)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	info, err := Probe(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsAnimated || info.Format != "gif" {
		t.Fatalf("unexpected info %+v", info)
	}
	img, err := Decode(buf.Bytes(), info, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 150 {
		t.Fatalf("unexpected first frame bounds %v", img.Bounds())
	}
}

func TestWebPStillVariants(t *testing.T) {
	for _, name := range []string{"still.webp", "still_lossless.webp", "still_alpha.webp"} {
		t.Run(name, func(t *testing.T) {
			data := readFixture(t, name)
			info, err := Probe(data)
			if err != nil {
				t.Fatal(err)
			}
			if info.IsAnimated || info.Width != 300 || info.Height != 200 {
				t.Fatalf("unexpected info %+v", info)
			}
			if _, err := Decode(data, info, 1<<30); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAnimatedWebPFirstFrame(t *testing.T) {
	for _, name := range []string{"anim.webp", "anim_alpha.webp"} {
		t.Run(name, func(t *testing.T) {
			data := readFixture(t, name)
			info, err := Probe(data)
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsAnimated {
				t.Fatal("expected animated webp")
			}
			img, err := Decode(data, info, 1<<30)
			if err != nil {
				t.Fatalf("animated webp must decode its first frame: %v", err)
			}
			if img.Bounds().Dx() <= 0 {
				t.Fatal("empty frame")
			}
			_ = PerceptualHash(img, info.Orientation)
		})
	}
}

func TestWebPCanvasMismatchRejected(t *testing.T) {
	data := readFixture(t, "still_lossless.webp")
	chunks, err := webpChunks(data, 12)
	if err != nil {
		t.Fatal(err)
	}
	var vp8l []byte
	for _, c := range chunks {
		if c.id == "VP8L" {
			vp8l = c.data
		}
	}
	if vp8l == nil {
		t.Fatal("fixture has no VP8L chunk")
	}
	var body bytes.Buffer
	body.WriteString("WEBP")
	vp8x := make([]byte, 10)
	vp8x[4] = 9
	vp8x[7] = 9
	writeChunk(&body, "VP8X", vp8x)
	writeChunk(&body, "VP8L", vp8l)
	var out bytes.Buffer
	out.WriteString("RIFF")
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(body.Len()))
	out.Write(size[:])
	out.Write(body.Bytes())

	if _, err := Probe(out.Bytes()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt for canvas mismatch, got %v", err)
	}
}

func TestJPEGOrientationShortSegmentDoesNotPanic(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x04, 'E', 'x', 'i', 'f', 0, 0, 0xFF, 0xD9}
	if got := exifOrientation(data); got != 1 {
		t.Fatalf("expected default orientation, got %d", got)
	}
}

func TestReadAllLimit(t *testing.T) {
	if _, err := ReadAll(bytes.NewReader(make([]byte, 11)), 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if d, err := ReadAll(bytes.NewReader(make([]byte, 10)), 10); err != nil || len(d) != 10 {
		t.Fatalf("unexpected result %v %d", err, len(d))
	}
}

func syntheticJPEG(sof byte, w, h, comps, scans int) []byte {
	b := []byte{0xFF, 0xD8}
	sofLen := 8 + 3*comps
	b = append(b, 0xFF, sof, byte(sofLen>>8), byte(sofLen), 8, byte(h>>8), byte(h), byte(w>>8), byte(w), byte(comps))
	for c := 1; c <= comps; c++ {
		b = append(b, byte(c), 0x11, 0)
	}
	for i := 0; i < scans; i++ {
		b = append(b, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0x00)
	}
	return append(b, 0xFF, 0xD9)
}

func TestProbeRejectsJPEGWithTooManyScans(t *testing.T) {
	if _, err := Probe(syntheticJPEG(0xC2, 1000, 1000, 3, 10)); err != nil {
		t.Fatalf("a normal progressive JPEG must pass: %v", err)
	}
	if _, err := Probe(syntheticJPEG(0xC2, 1000, 1000, 3, MaxJPEGScans+1)); !errors.Is(err, ErrTooComplex) {
		t.Fatalf("expected ErrTooComplex for %d scans, got %v", MaxJPEGScans+1, err)
	}
}

func TestDecodeBytesEstimates(t *testing.T) {
	info, err := Probe(syntheticJPEG(0xC2, 9000, 9000, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(3 * 1125 * 1125 * 320); info.DecodeBytes != want {
		t.Fatalf("progressive JPEG estimate = %d, want %d", info.DecodeBytes, want)
	}
	info, err = Probe(syntheticJPEG(0xC0, 9000, 9000, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(3 * 1125 * 1125 * 64); info.DecodeBytes != want {
		t.Fatalf("baseline JPEG estimate = %d, want %d", info.DecodeBytes, want)
	}

	var buf bytes.Buffer
	img := image.NewNRGBA64(image.Rect(0, 0, 64, 32))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	info, err = Probe(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(64 * 32 * 8); info.DecodeBytes != want {
		t.Fatalf("16-bit PNG estimate = %d, want %d", info.DecodeBytes, want)
	}
}
