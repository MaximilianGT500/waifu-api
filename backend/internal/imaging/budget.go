package imaging

import (
	"bytes"
	"encoding/binary"
)

type jpegComponent struct{ h, v int }

type jpegMeta struct {
	progressive bool
	components  []jpegComponent
	scans       int
}

func parseJPEG(data []byte) (jpegMeta, error) {
	var m jpegMeta
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return m, ErrCorrupt
	}
	i := 2
	for i < len(data) {
		if data[i] != 0xFF {
			return m, ErrCorrupt
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		switch {
		case marker == 0xD9:
			return m, nil
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8):
			continue
		}
		if i+2 > len(data) {
			break
		}
		length := int(data[i])<<8 | int(data[i+1])
		if length < 2 || i+length > len(data) {
			break
		}
		seg := data[i+2 : i+length]
		i += length

		switch marker {
		case 0xC0, 0xC1, 0xC2:
			if len(seg) < 6 {
				return m, ErrCorrupt
			}
			n := int(seg[5])
			if n < 1 || n > 4 || len(seg) < 6+3*n {
				return m, ErrCorrupt
			}
			m.progressive = marker == 0xC2
			m.components = m.components[:0]
			for c := 0; c < n; c++ {
				hv := seg[6+3*c+1]
				h, v := int(hv>>4), int(hv&0x0F)
				if h < 1 || h > 4 || v < 1 || v > 4 {
					return m, ErrCorrupt
				}
				m.components = append(m.components, jpegComponent{h: h, v: v})
			}
		case 0xDA:
			m.scans++
			if m.scans > MaxJPEGScans {
				return m, ErrTooComplex
			}
			for i < len(data) {
				if data[i] == 0xFF && i+1 < len(data) {
					next := data[i+1]
					if next == 0x00 || (next >= 0xD0 && next <= 0xD7) {
						i += 2
						continue
					}
					if next != 0xFF {
						break
					}
				}
				i++
			}
		}
	}
	return m, nil
}

func (m jpegMeta) decodeBytes(w, h int) int64 {
	if len(m.components) == 0 {
		return 3 * int64(w) * int64(h)
	}
	hmax, vmax := 1, 1
	for _, c := range m.components {
		hmax, vmax = max(hmax, c.h), max(vmax, c.v)
	}
	mxx := int64((w + 8*hmax - 1) / (8 * hmax))
	myy := int64((h + 8*vmax - 1) / (8 * vmax))
	var total int64
	for _, c := range m.components {
		blocks := mxx * myy * int64(c.h) * int64(c.v)
		total += blocks * 64
		if m.progressive {
			total += blocks * 64 * 4
		}
	}
	if len(m.components) == 4 {
		total += 4 * int64(w) * int64(h)
	}
	return total
}

func pngDecodeBytes(data []byte, px int64) int64 {
	if len(data) < 33 || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return 8 * px
	}
	bitDepth, colorType, interlace := data[24], data[25], data[28]
	transparency := false
	for pos := 8; pos+8 <= len(data); {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		typ := string(data[pos+4 : pos+8])
		if typ == "tRNS" {
			transparency = true
		}
		if typ == "IDAT" || typ == "IEND" || length < 0 || pos+12+length > len(data) {
			break
		}
		pos += 12 + length
	}
	var bpp int64
	switch colorType {
	case 0:
		switch {
		case transparency && bitDepth == 16:
			bpp = 8
		case transparency:
			bpp = 4
		case bitDepth == 16:
			bpp = 2
		default:
			bpp = 1
		}
	case 3:
		bpp = 1
	case 2:
		if bitDepth == 16 {
			bpp = 8
		} else {
			bpp = 4
		}
	default:
		if bitDepth == 16 {
			bpp = 8
		} else {
			bpp = 4
		}
	}
	total := bpp * px
	if interlace == 1 {
		total += total / 2
	}
	return total
}

func webpDecodeBytes(meta *webpMeta, canvasPx int64) int64 {
	px := canvasPx
	if meta.animated && meta.frameW > 0 && meta.frameH > 0 {
		px = int64(meta.frameW) * int64(meta.frameH)
	}
	if meta.lossless {
		return 5 * px
	}
	return px*3/2 + px
}
