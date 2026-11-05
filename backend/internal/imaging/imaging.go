package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"sort"

	"github.com/disintegration/imaging"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
	"golang.org/x/image/webp"
)

type Info struct {
	Width       int
	Height      int
	IsAnimated  bool
	Format      string
	Orientation int
	DecodeBytes int64
}

const MaxJPEGScans = 32

var (
	ErrUnsupportedFormat = errors.New("unsupported image format")
	ErrCorrupt           = errors.New("corrupt image")
	ErrTooManyPixels     = errors.New("image has too many pixels")
	ErrTooLarge          = errors.New("image file too large")
	ErrTooComplex        = errors.New("image too complex to decode")
)

func ReadAll(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, ErrTooLarge
	}
	return data, nil
}

func Probe(data []byte) (*Info, error) {
	cfg, name, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	switch name {
	case "jpeg", "png", "gif", "webp":
	default:
		return nil, ErrUnsupportedFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrCorrupt
	}
	info := &Info{
		Width:       cfg.Width,
		Height:      cfg.Height,
		Format:      name,
		IsAnimated:  name == "gif",
		Orientation: exifOrientation(data),
	}
	px := int64(cfg.Width) * int64(cfg.Height)
	switch name {
	case "webp":
		meta, err := parseWebP(data)
		if err != nil {
			return nil, err
		}
		info.IsAnimated = meta.animated
		if meta.animated {
			if meta.frameW <= 0 || meta.frameH <= 0 || meta.frameW > cfg.Width || meta.frameH > cfg.Height {
				return nil, ErrCorrupt
			}
		} else if meta.bitstreamW != cfg.Width || meta.bitstreamH != cfg.Height {
			return nil, ErrCorrupt
		}
		info.DecodeBytes = webpDecodeBytes(meta, px)
	case "jpeg":
		meta, err := parseJPEG(data)
		if err != nil {
			return nil, err
		}
		info.DecodeBytes = meta.decodeBytes(cfg.Width, cfg.Height)
	case "png":
		info.DecodeBytes = pngDecodeBytes(data, px)
	case "gif":
		info.DecodeBytes = px
	}
	return info, nil
}

func Decode(data []byte, info *Info, maxPixels int64) (image.Image, error) {
	if maxPixels > 0 && int64(info.Width)*int64(info.Height) > maxPixels {
		return nil, ErrTooManyPixels
	}
	r := bytes.NewReader(data)
	var (
		img image.Image
		err error
	)
	switch info.Format {
	case "gif":
		img, err = gif.Decode(r)
	case "webp":
		if info.IsAnimated {
			var frame []byte
			frame, err = firstWebPFrame(data)
			if err == nil {
				img, err = webp.Decode(bytes.NewReader(frame))
			}
		} else {
			img, err = webp.Decode(r)
		}
	case "jpeg":
		img, err = jpeg.Decode(r)
	case "png":
		img, err = png.Decode(r)
	default:
		return nil, ErrUnsupportedFormat
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, ErrCorrupt
	}
	return img, nil
}

func ContentType(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	}
	return "application/octet-stream"
}

func Extension(format, clientExt string) string {
	switch format {
	case "jpeg":
		if clientExt == ".jpeg" || clientExt == ".jpg" {
			return clientExt
		}
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "webp":
		return ".webp"
	}
	return ""
}

func ValidateDimensions(info *Info, minW, minH, maxW, maxH, animMinW, animMinH, animMaxW, animMaxH int) error {
	minWidth, minHeight, maxWidth, maxHeight := minW, minH, maxW, maxH
	suffix := ""
	if info.IsAnimated {
		suffix = " for animated images"
		if animMinW > 0 {
			minWidth = animMinW
		}
		if animMinH > 0 {
			minHeight = animMinH
		}
		if animMaxW > 0 {
			maxWidth = animMaxW
		}
		if animMaxH > 0 {
			maxHeight = animMaxH
		}
	}
	if info.Width < minWidth || info.Height < minHeight {
		return fmt.Errorf("Image resolution is too low (%dx%d). Minimum is %dx%d%s.",
			info.Width, info.Height, minWidth, minHeight, suffix)
	}
	if info.Width > maxWidth || info.Height > maxHeight {
		return fmt.Errorf("Image resolution is too high (%dx%d). Maximum is %dx%d%s.",
			info.Width, info.Height, maxWidth, maxHeight, suffix)
	}
	return nil
}

func DominantColor(img image.Image) string {
	thumb := imaging.Resize(img, 1, 1, imaging.CatmullRom)
	c := thumb.At(0, 0)
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

const phashSize = 64

func PerceptualHash(img image.Image, orientation int) uint64 {
	const size = phashSize
	resized := image.Image(imaging.Resize(img, size, size, imaging.CatmullRom))
	resized = applyOrientation(resized, orientation)

	gray := image.NewGray(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, _ := resized.At(x, y).RGBA()
			rf, gf, bf := float64(r>>8), float64(g>>8), float64(b>>8)
			yVal := math.Round(0.299*rf + 0.587*gf + 0.114*bf)
			gray.Set(x, y, color.Gray{Y: uint8(yVal)})
		}
	}

	rows := make([][]float64, size)
	for y := 0; y < size; y++ {
		seq := make([]float64, size)
		for x := 0; x < size; x++ {
			seq[x] = float64(gray.GrayAt(x, y).Y)
		}
		rows[y] = dct1D(seq, size)
	}

	matrix := make([][]float64, 8)
	for x := 0; x < 8; x++ {
		seq := make([]float64, size)
		for y := 0; y < size; y++ {
			seq[y] = rows[y][x]
		}
		matrix[x] = dct1D(seq, 8)
	}

	var top8x8 [64]float64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			top8x8[y*8+x] = matrix[y][x]
		}
	}

	sorted := append([]float64(nil), top8x8[:]...)
	sort.Float64s(sorted)
	median := (sorted[31] + sorted[32]) / 2.0

	var hash uint64
	mask := uint64(1) << 63
	for i := 0; i < size; i++ {
		if top8x8[i] > median {
			hash |= mask
		}
		mask >>= 1
	}
	return hash
}

func dct1D(v []float64, limit int) []float64 {
	out := make([]float64, limit)
	for c := 0; c < limit; c++ {
		var sum float64
		for i := 0; i < phashSize; i++ {
			sum += v[i] * math.Cos(float64(2*i+1)*float64(c)*math.Pi/float64(2*phashSize))
		}
		sum *= math.Sqrt(2.0 / phashSize)
		if c == 0 {
			sum *= 1.0 / math.Sqrt(2)
		}
		out[c] = sum
	}
	return out
}

func applyOrientation(img image.Image, orient int) image.Image {
	switch orient {
	case 2:
		return imaging.FlipH(img)
	case 3:
		return imaging.Rotate180(img)
	case 4:
		return imaging.FlipV(img)
	case 5:
		return imaging.Transpose(img)
	case 6:
		return imaging.Rotate90(img)
	case 7:
		return imaging.Transverse(img)
	case 8:
		return imaging.Rotate270(img)
	default:
		return img
	}
}

func exifOrientation(data []byte) int {
	if len(data) > 4 && data[0] == 0xFF && data[1] == 0xD8 {
		return jpegOrientation(data)
	}
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		return pngOrientation(data)
	}
	if len(data) > 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return webpOrientation(data)
	}
	return 1
}

func jpegOrientation(data []byte) int {
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		i += 2
		if marker == 0xDA || marker == 0xD9 {
			break
		}
		if i+2 > len(data) {
			break
		}
		size := int(data[i])<<8 | int(data[i+1])
		if size < 2 || i+size > len(data) {
			break
		}
		if marker == 0xE1 && size >= 8 && string(data[i+2:i+8]) == "Exif\x00\x00" {
			return tiffOrientation(data[i+8 : i+size])
		}
		i += size
	}
	return 1
}

func pngOrientation(data []byte) int {
	pos := 8
	for pos+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		typ := string(data[pos+4 : pos+8])
		if length < 0 {
			break
		}
		if typ == "eXIf" && pos+8+length <= len(data) {
			return tiffOrientation(data[pos+8 : pos+8+length])
		}
		if typ == "IEND" {
			break
		}
		pos += 8 + length + 4
		if pos > len(data) || pos < 0 {
			break
		}
	}
	return 1
}

func webpOrientation(data []byte) int {
	pos := 12
	for pos+8 <= len(data) {
		typ := string(data[pos : pos+4])
		length := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if length < 0 {
			break
		}
		if typ == "EXIF" && pos+8+length <= len(data) {
			return tiffOrientation(data[pos+8 : pos+8+length])
		}
		if pos+8+length+(length&1) > len(data) {
			break
		}
		pos += 8 + length + (length & 1)
	}
	return 1
}

func tiffOrientation(exif []byte) int {
	if len(exif) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(exif, []byte("II*\x00")):
		order = binary.LittleEndian
	case bytes.HasPrefix(exif, []byte("MM\x00*")):
		order = binary.BigEndian
	default:
		return 1
	}
	ifdOff := int(order.Uint32(exif[4:8]))
	if ifdOff < 0 || ifdOff+2 > len(exif) {
		return 1
	}
	n := int(order.Uint16(exif[ifdOff : ifdOff+2]))
	for i := 0; i < n && ifdOff+2+12*(i+1) <= len(exif); i++ {
		entry := exif[ifdOff+2+12*i : ifdOff+2+12*(i+1)]
		tag := order.Uint16(entry[0:2])
		typ := order.Uint16(entry[2:4])
		count := order.Uint32(entry[4:8])
		if tag == 0x0112 && typ == 3 && count == 1 {
			if o := int(order.Uint16(entry[8:10])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

type webpChunk struct {
	id   string
	data []byte
}

type webpMeta struct {
	animated               bool
	lossless               bool
	bitstreamW, bitstreamH int
	frameW, frameH         int
}

func webpChunks(data []byte, start int) ([]webpChunk, error) {
	var out []webpChunk
	pos := start
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		begin := int64(pos + 8)
		end := begin + size
		if end > int64(len(data)) {
			return nil, ErrCorrupt
		}
		out = append(out, webpChunk{id: id, data: data[begin:end]})
		pos = int(end + size&1)
		if len(out) > 100000 {
			return nil, ErrCorrupt
		}
	}
	return out, nil
}

func parseWebP(data []byte) (*webpMeta, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, ErrCorrupt
	}
	chunks, err := webpChunks(data, 12)
	if err != nil {
		return nil, err
	}
	meta := &webpMeta{}
	foundBitstream := false
	foundFrame := false
	for _, c := range chunks {
		switch c.id {
		case "ANIM":
			meta.animated = true
		case "ANMF":
			if foundFrame {
				continue
			}
			w, h, sub, err := parseANMF(c.data)
			if err != nil {
				return nil, err
			}
			meta.frameW, meta.frameH = w, h
			for _, sc := range sub {
				if sc.id == "VP8L" {
					meta.lossless = true
					break
				}
			}
			foundFrame = true
		case "VP8 ", "VP8L":
			if foundBitstream {
				continue
			}
			w, h, err := bitstreamDims(c.id, c.data)
			if err != nil {
				return nil, err
			}
			meta.bitstreamW, meta.bitstreamH = w, h
			if !meta.animated {
				meta.lossless = c.id == "VP8L"
			}
			foundBitstream = true
		}
	}
	if meta.animated && !foundFrame {
		return nil, ErrCorrupt
	}
	if !meta.animated && !foundBitstream {
		return nil, ErrCorrupt
	}
	return meta, nil
}

func bitstreamDims(id string, payload []byte) (int, int, error) {
	switch id {
	case "VP8 ":
		d := vp8.NewDecoder()
		d.Init(bytes.NewReader(payload), len(payload))
		fh, err := d.DecodeFrameHeader()
		if err != nil {
			return 0, 0, ErrCorrupt
		}
		return fh.Width, fh.Height, nil
	case "VP8L":
		cfg, err := vp8l.DecodeConfig(bytes.NewReader(payload))
		if err != nil {
			return 0, 0, ErrCorrupt
		}
		return cfg.Width, cfg.Height, nil
	}
	return 0, 0, ErrCorrupt
}

func u24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }

func parseANMF(payload []byte) (int, int, []webpChunk, error) {
	if len(payload) < 16+8 {
		return 0, 0, nil, ErrCorrupt
	}
	w := u24(payload[6:9]) + 1
	h := u24(payload[9:12]) + 1
	sub, err := webpChunks(payload, 16)
	if err != nil {
		return 0, 0, nil, err
	}
	var bitstream *webpChunk
	for i := range sub {
		if sub[i].id == "VP8 " || sub[i].id == "VP8L" {
			bitstream = &sub[i]
			break
		}
	}
	if bitstream == nil {
		return 0, 0, nil, ErrCorrupt
	}
	bw, bh, err := bitstreamDims(bitstream.id, bitstream.data)
	if err != nil {
		return 0, 0, nil, err
	}
	if bw != w || bh != h {
		return 0, 0, nil, ErrCorrupt
	}
	return w, h, sub, nil
}

func firstWebPFrame(data []byte) ([]byte, error) {
	chunks, err := webpChunks(data, 12)
	if err != nil {
		return nil, err
	}
	for _, c := range chunks {
		if c.id != "ANMF" {
			continue
		}
		w, h, sub, err := parseANMF(c.data)
		if err != nil {
			return nil, err
		}
		var alph, bitstream *webpChunk
		for i := range sub {
			switch sub[i].id {
			case "ALPH":
				if alph == nil {
					alph = &sub[i]
				}
			case "VP8 ", "VP8L":
				if bitstream == nil {
					bitstream = &sub[i]
				}
			}
		}
		var body bytes.Buffer
		body.WriteString("WEBP")
		if alph != nil && bitstream.id == "VP8 " {
			vp8x := make([]byte, 10)
			vp8x[0] = 0x10
			wm, hm := w-1, h-1
			vp8x[4], vp8x[5], vp8x[6] = byte(wm), byte(wm>>8), byte(wm>>16)
			vp8x[7], vp8x[8], vp8x[9] = byte(hm), byte(hm>>8), byte(hm>>16)
			writeChunk(&body, "VP8X", vp8x)
			writeChunk(&body, "ALPH", alph.data)
		}
		writeChunk(&body, bitstream.id, bitstream.data)

		var out bytes.Buffer
		out.WriteString("RIFF")
		var size [4]byte
		binary.LittleEndian.PutUint32(size[:], uint32(body.Len()))
		out.Write(size[:])
		out.Write(body.Bytes())
		return out.Bytes(), nil
	}
	return nil, ErrCorrupt
}

func writeChunk(buf *bytes.Buffer, id string, payload []byte) {
	buf.WriteString(id)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	buf.Write(size[:])
	buf.Write(payload)
	if len(payload)&1 == 1 {
		buf.WriteByte(0)
	}
}
