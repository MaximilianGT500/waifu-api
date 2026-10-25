package imaging

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
	"sort"

	_ "image/jpeg"
	_ "image/png"

	"github.com/disintegration/imaging"
	"golang.org/x/image/webp"
)

type Info struct {
	Width       int
	Height      int
	IsAnimated  bool
	Format      string
	Orientation int
}

func Decode(r io.Reader) (*Info, image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, nil, err
	}

	cfg, fmtName, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("unsupported or corrupt image: %w", err)
	}

	var img image.Image
	switch fmtName {
	case "gif":
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
		if len(g.Image) == 0 {
			return nil, nil, fmt.Errorf("corrupt gif: no frames")
		}

		img = g.Image[0]
	case "webp":
		img, err = webp.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
	default:
		img, err = imaging.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
	}

	isAnimated := fmtName == "gif"
	if fmtName == "webp" {
		isAnimated = webpIsAnimated(data)
	}

	return &Info{
		Width:       cfg.Width,
		Height:      cfg.Height,
		IsAnimated:  isAnimated,
		Format:      fmtName,
		Orientation: exifOrientation(data),
	}, img, nil
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
		if marker == 0xE1 && i+2+6 <= len(data) && string(data[i+2:i+2+6]) == "Exif\x00\x00" {
			return tiffOrientation(data[i+2+6 : i+size])
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
		if typ == "eXIf" && pos+8+length <= len(data) {
			return tiffOrientation(data[pos+8 : pos+8+length])
		}
		if typ == "IEND" {
			break
		}
		pos += 8 + length + 4
		if pos > len(data) {
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

func webpIsAnimated(data []byte) bool {
	pos := 12
	for pos+8 <= len(data) {
		typ := string(data[pos : pos+4])
		length := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if typ == "ANIM" {
			return true
		}
		if typ == "VP8 " || typ == "VP8L" {
			return false
		}
		if pos+8+length+(length&1) > len(data) {
			break
		}
		pos += 8 + length + (length & 1)
	}
	return false
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
	if ifdOff+2 > len(exif) {
		return 1
	}
	n := int(order.Uint16(exif[ifdOff : ifdOff+2]))
	for i := 0; i < n && ifdOff+2+12*(i+1) <= len(exif); i++ {
		entry := exif[ifdOff+2+12*i : ifdOff+2+12*(i+1)]
		tag := order.Uint16(entry[0:2])
		typ := order.Uint16(entry[2:4])
		count := order.Uint32(entry[4:8])
		if tag == 0x0112 && typ == 3 && count == 1 {
			return int(order.Uint16(entry[8:10]))
		}
	}
	return 1
}
