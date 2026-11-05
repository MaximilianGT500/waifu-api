package imaging

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

func StripMetadata(data []byte, info *Info) ([]byte, error) {
	switch info.Format {
	case "jpeg":
		return stripJPEG(data, info.Orientation)
	case "png":
		return stripPNG(data, info.Orientation)
	case "webp":
		return stripWebP(data, info.Orientation)
	}
	return data, nil
}

func keepsOrientation(orientation int) bool { return orientation >= 2 && orientation <= 8 }

func orientationTIFF(orientation int) []byte {
	b := make([]byte, 26)
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], 1)
	binary.LittleEndian.PutUint16(b[10:12], 0x0112)
	binary.LittleEndian.PutUint16(b[12:14], 3)
	binary.LittleEndian.PutUint32(b[14:18], 1)
	binary.LittleEndian.PutUint16(b[18:20], uint16(orientation))
	return b
}

func isJPEGRestart(m byte) bool { return m >= 0xD0 && m <= 0xD7 }

func dropJPEGSegment(marker byte, payload []byte) bool {
	switch marker {
	case 0xE1, 0xED, 0xFE:
		return true
	case 0xE2:
		return bytes.HasPrefix(payload, []byte("MPF\x00"))
	}
	return false
}

func stripJPEG(data []byte, orientation int) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, ErrCorrupt
	}
	out := make([]byte, 0, len(data))
	out = append(out, 0xFF, 0xD8)
	exifDone := !keepsOrientation(orientation)
	writeExif := func() {
		if exifDone {
			return
		}
		payload := append([]byte("Exif\x00\x00"), orientationTIFF(orientation)...)
		size := len(payload) + 2
		out = append(out, 0xFF, 0xE1, byte(size>>8), byte(size))
		out = append(out, payload...)
		exifDone = true
	}
	sawScan := false
	i := 2
	for i < len(data) {
		if data[i] != 0xFF {
			i++
			continue
		}
		if i+1 >= len(data) {
			break
		}
		marker := data[i+1]
		switch {
		case marker == 0xFF:
			i++
			continue
		case marker == 0xD9:
			if !sawScan {
				return nil, ErrCorrupt
			}
			return append(out, 0xFF, 0xD9), nil
		case marker == 0x01 || isJPEGRestart(marker) || marker == 0x00:
			i += 2
			continue
		}
		if i+4 > len(data) {
			break
		}
		size := int(data[i+2])<<8 | int(data[i+3])
		if size < 2 || i+2+size > len(data) {
			return nil, ErrCorrupt
		}
		if marker != 0xE0 {
			writeExif()
		}
		if !dropJPEGSegment(marker, data[i+4:i+2+size]) {
			out = append(out, data[i:i+2+size]...)
		}
		i += 2 + size
		if marker != 0xDA {
			continue
		}
		sawScan = true
		j := i
		for j+1 < len(data) && !(data[j] == 0xFF && data[j+1] != 0x00 && !isJPEGRestart(data[j+1])) {
			j++
		}
		out = append(out, data[i:j]...)
		i = j
	}
	return nil, ErrCorrupt
}

func appendPNGChunk(out []byte, typ string, payload []byte) []byte {
	out = binary.BigEndian.AppendUint32(out, uint32(len(payload)))
	start := len(out)
	out = append(out, typ...)
	out = append(out, payload...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[start:]))
}

func stripPNG(data []byte, orientation int) ([]byte, error) {
	signature := []byte("\x89PNG\r\n\x1a\n")
	if !bytes.HasPrefix(data, signature) {
		return nil, ErrCorrupt
	}
	out := make([]byte, 0, len(data))
	out = append(out, signature...)
	pos := len(signature)
	for pos+12 <= len(data) {
		length := int64(binary.BigEndian.Uint32(data[pos : pos+4]))
		end := int64(pos) + 12 + length
		if length > 0x7fffffff || end > int64(len(data)) {
			return nil, ErrCorrupt
		}
		typ := string(data[pos+4 : pos+8])
		chunk := data[pos:end]
		pos = int(end)
		switch typ {
		case "tEXt", "zTXt", "iTXt", "eXIf", "tIME":
			continue
		}
		out = append(out, chunk...)
		switch typ {
		case "IHDR":
			if keepsOrientation(orientation) {
				out = appendPNGChunk(out, "eXIf", orientationTIFF(orientation))
			}
		case "IEND":
			return out, nil
		}
	}
	return nil, ErrCorrupt
}

func stripWebP(data []byte, orientation int) ([]byte, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, ErrCorrupt
	}
	end := 8 + int64(binary.LittleEndian.Uint32(data[4:8]))
	if end < 12 || end > int64(len(data)) {
		return nil, ErrCorrupt
	}
	chunks, err := webpChunks(data[:end], 12)
	if err != nil {
		return nil, err
	}
	keepExif := keepsOrientation(orientation)
	extended := false
	var body bytes.Buffer
	for _, c := range chunks {
		switch c.id {
		case "EXIF", "XMP ":
			continue
		case "VP8X":
			if len(c.data) < 10 {
				return nil, ErrCorrupt
			}
			extended = true
			header := append([]byte(nil), c.data...)
			header[0] &^= 0x08 | 0x04
			if keepExif {
				header[0] |= 0x08
			}
			writeChunk(&body, "VP8X", header)
			continue
		}
		writeChunk(&body, c.id, c.data)
	}
	if keepExif && extended {
		writeChunk(&body, "EXIF", orientationTIFF(orientation))
	}
	out := make([]byte, 0, 12+body.Len())
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(4+body.Len()))
	out = append(out, "WEBP"...)
	return append(out, body.Bytes()...), nil
}
