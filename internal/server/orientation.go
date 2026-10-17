package server

import (
	"bytes"
	"encoding/binary"
	"image"
)

// jpegOrientation reads only the TIFF orientation tag in JPEG APP1 segments.
// Malformed or absent metadata is ignored; image.Decode validates image pixels.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for pos := 2; pos+4 <= len(data); {
		if data[pos] != 0xff {
			return 1
		}
		marker := data[pos+1]
		if marker == 0xda || marker == 0xd9 {
			return 1
		}
		if marker == 0xff {
			pos++
			continue
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			pos += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if n < 2 || pos+2+n > len(data) {
			return 1
		}
		segment := data[pos+4 : pos+2+n]
		if marker == 0xe1 && len(segment) >= 14 && bytes.Equal(segment[:6], []byte("Exif\x00\x00")) {
			tiff := segment[6:]
			var order binary.ByteOrder
			switch string(tiff[:2]) {
			case "II":
				order = binary.LittleEndian
			case "MM":
				order = binary.BigEndian
			default:
				return 1
			}
			if order.Uint16(tiff[2:4]) != 42 {
				return 1
			}
			offset := uint64(order.Uint32(tiff[4:8]))
			if offset+2 > uint64(len(tiff)) {
				return 1
			}
			count := uint64(order.Uint16(tiff[offset : offset+2]))
			for i := uint64(0); i < count; i++ {
				start := offset + 2 + i*12
				if start+12 > uint64(len(tiff)) {
					return 1
				}
				entry := tiff[start : start+12]
				if order.Uint16(entry[:2]) == 0x0112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
					v := int(order.Uint16(entry[8:10]))
					if v >= 1 && v <= 8 {
						return v
					}
					return 1
				}
			}
		}
		pos += 2 + n
	}
	return 1
}

// Orient after resizing to avoid allocating a second full-resolution image.
func orientImage(src *image.RGBA, orientation int) *image.RGBA {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	ow, oh := w, h
	if orientation >= 5 {
		ow, oh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			sx, sy := x, y
			switch orientation {
			case 2:
				sx = w - 1 - x
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sy = h - 1 - y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}
