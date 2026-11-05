package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

var secret = []byte("SECRET-METADATA")

func samePixels(t *testing.T, a, b image.Image) {
	t.Helper()
	if a.Bounds() != b.Bounds() {
		t.Fatalf("bounds differ: %v vs %v", a.Bounds(), b.Bounds())
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Fatalf("pixel %d,%d differs", x, y)
			}
		}
	}
}

func tiffWithOrientation(order binary.ByteOrder, orientation uint16, extra []byte) []byte {
	var b bytes.Buffer
	if order == binary.BigEndian {
		b.WriteString("MM\x00*")
	} else {
		b.WriteString("II*\x00")
	}
	put32 := func(v uint32) { _ = binary.Write(&b, order, v) }
	put16 := func(v uint16) { _ = binary.Write(&b, order, v) }
	put32(8)
	put16(2)
	put16(0x0112)
	put16(3)
	put32(1)
	put16(orientation)
	put16(0)
	put16(0x8825)
	put16(4)
	put32(1)
	put32(38)
	put32(0)
	b.Write(extra)
	return b.Bytes()
}

func jpegSegment(marker byte, payload []byte) []byte {
	size := len(payload) + 2
	return append([]byte{0xFF, marker, byte(size >> 8), byte(size)}, payload...)
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 5), uint8(y * 7), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStripJPEGRemovesMetadataAndKeepsOrientation(t *testing.T) {
	plain := testJPEG(t)
	var in []byte
	in = append(in, 0xFF, 0xD8)
	in = append(in, jpegSegment(0xE1, append([]byte("Exif\x00\x00"), tiffWithOrientation(binary.BigEndian, 6, secret)...))...)
	in = append(in, jpegSegment(0xE1, append([]byte("http://ns.adobe.com/xap/1.0/\x00"), secret...))...)
	in = append(in, jpegSegment(0xED, append([]byte("Photoshop 3.0\x00"), secret...))...)
	in = append(in, jpegSegment(0xFE, secret)...)
	in = append(in, jpegSegment(0xE2, append([]byte("MPF\x00"), secret...))...)
	in = append(in, plain[2:]...)
	in = append(in, secret...)

	info, err := Probe(in)
	if err != nil || info.Orientation != 6 {
		t.Fatalf("probe: %v, orientation %v", err, info)
	}
	out, err := StripMetadata(in, info)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, secret) {
		t.Fatal("metadata survived")
	}
	if got := exifOrientation(out); got != 6 {
		t.Fatalf("orientation must be kept, got %d", got)
	}
	if !bytes.HasSuffix(out, []byte{0xFF, 0xD9}) {
		t.Fatal("data after the end of the image must be dropped")
	}
	want, err := jpeg.Decode(bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("stripped JPEG must decode: %v", err)
	}
	samePixels(t, want, got)

	upright := append([]byte{0xFF, 0xD8}, jpegSegment(0xE1, append([]byte("Exif\x00\x00"), tiffWithOrientation(binary.LittleEndian, 1, secret)...))...)
	upright = append(upright, plain[2:]...)
	info, _ = Probe(upright)
	out, err = StripMetadata(upright, info)
	if err != nil || bytes.Contains(out, []byte("Exif")) || !bytes.Equal(out, plain) {
		t.Fatalf("an upright JPEG must lose its whole EXIF block (err %v)", err)
	}
}

func TestStripJPEGKeepsMultiScanFiles(t *testing.T) {
	in := syntheticJPEG(0xC2, 64, 64, 3, 5)
	out, err := stripJPEG(in, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, out) {
		t.Fatal("a JPEG without metadata must stay byte-identical")
	}
	if _, err := stripJPEG(in[:len(in)-2], 1); err == nil {
		t.Fatal("a JPEG without end marker must be reported")
	}
}

func TestStripPNGRemovesTextAndKeepsOrientation(t *testing.T) {
	plain := encodePNG(t, 40, 20)
	var buf bytes.Buffer
	buf.Write(plain[:33])
	writePNGChunk(&buf, "tEXt", append([]byte("Comment\x00"), secret...))
	writePNGChunk(&buf, "iTXt", append([]byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00"), secret...))
	writePNGChunk(&buf, "zTXt", append([]byte("Raw profile\x00\x00"), secret...))
	writePNGChunk(&buf, "tIME", []byte{0x07, 0xEA, 10, 30, 12, 0, 0})
	writePNGChunk(&buf, "eXIf", tiffWithOrientation(binary.BigEndian, 3, secret))
	buf.Write(plain[33:])
	buf.Write(secret)
	in := buf.Bytes()

	info, err := Probe(in)
	if err != nil || info.Orientation != 3 {
		t.Fatalf("probe: %v, %+v", err, info)
	}
	out, err := StripMetadata(in, info)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, secret) || bytes.Contains(out, []byte("tIME")) {
		t.Fatal("metadata survived")
	}
	if got := exifOrientation(out); got != 3 {
		t.Fatalf("orientation must be kept, got %d", got)
	}
	want, _ := png.Decode(bytes.NewReader(plain))
	got, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("stripped PNG must decode (CRCs): %v", err)
	}
	samePixels(t, want, got)
}

func withWebPMetadata(t *testing.T, plain []byte, orientation uint16) []byte {
	t.Helper()
	chunks, err := webpChunks(plain, 12)
	if err != nil || len(chunks) == 0 || chunks[0].id != "VP8X" {
		t.Fatalf("fixture must be an extended WebP: %v", err)
	}
	var body bytes.Buffer
	for _, c := range chunks {
		data := c.data
		if c.id == "VP8X" {
			data = append([]byte(nil), c.data...)
			data[0] |= 0x08 | 0x04
		}
		writeChunk(&body, c.id, data)
	}
	writeChunk(&body, "EXIF", tiffWithOrientation(binary.LittleEndian, orientation, secret))
	writeChunk(&body, "XMP ", secret)
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+body.Len()))...)
	out = append(out, "WEBP"...)
	return append(out, body.Bytes()...)
}

func TestStripWebPRemovesExifAndXMP(t *testing.T) {
	plain := readFixture(t, "still_alpha.webp")
	in := withWebPMetadata(t, plain, 8)
	info, err := Probe(in)
	if err != nil || info.Orientation != 8 {
		t.Fatalf("probe: %v, %+v", err, info)
	}
	out, err := StripMetadata(in, info)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, secret) || bytes.Contains(out, []byte("XMP ")) {
		t.Fatal("metadata survived")
	}
	if got := exifOrientation(out); got != 8 {
		t.Fatalf("orientation must be kept, got %d", got)
	}
	chunks, err := webpChunks(out, 12)
	if err != nil || chunks[0].data[0]&0x04 != 0 || chunks[0].data[0]&0x08 == 0 {
		t.Fatalf("VP8X flags must match the remaining chunks: %v", err)
	}
	if size := int(binary.LittleEndian.Uint32(out[4:8])); size+8 != len(out) {
		t.Fatalf("RIFF size %d does not match the file length %d", size, len(out))
	}
	want, _ := webp.Decode(bytes.NewReader(plain))
	got, err := webp.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("stripped WebP must decode: %v", err)
	}
	samePixels(t, want, got)

	anim := withWebPMetadata(t, readFixture(t, "anim.webp"), 1)
	info, err = Probe(anim)
	if err != nil {
		t.Fatal(err)
	}
	out, err = StripMetadata(anim, info)
	if err != nil || bytes.Contains(out, secret) || bytes.Contains(out, []byte("EXIF")) {
		t.Fatalf("animated WebP: %v", err)
	}
	if _, err := Probe(out); err != nil {
		t.Fatalf("stripped animated WebP must stay valid: %v", err)
	}
	if _, err := Decode(out, info, 0); err != nil {
		t.Fatalf("first frame of the stripped animation must decode: %v", err)
	}
}
