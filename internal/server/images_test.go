package server

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestCompressImage(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2400, 1200))
	src.SetNRGBA(1200, 600, color.NRGBA{R: 255, A: 255})
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatal(err)
	}
	data, w, h, err := compressImage(raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if w != 1920 || h != 960 {
		t.Fatalf("got %dx%d", w, h)
	}
	out, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := out.At(0, 0).RGBA()
	if r < 64000 || g < 64000 || b < 64000 {
		t.Fatal("transparent pixels must be flattened onto white")
	}
	if _, _, _, err := compressImage([]byte("<svg onload=alert(1)>")); err == nil {
		t.Fatal("non-raster input accepted")
	}
}

func TestPostValidation(t *testing.T) {
	valid := postInput{Title: "  留住时光  ", Body: "这是一段记忆", Category: "daily", Tags: []string{"生活", "生活"}}
	if err := validatePost(&valid); err != nil {
		t.Fatal(err)
	}
	if valid.Title != "留住时光" || len(valid.Tags) != 1 {
		t.Fatal("normalization failed")
	}
	for _, in := range []postInput{{Title: "", Body: "a", Category: "daily"}, {Title: "a", Body: "", Category: "daily"}, {Title: "a", Body: "b", Category: "private"}} {
		if validatePost(&in) == nil {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestJPEGOrientation(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, src, nil); err != nil {
		t.Fatal(err)
	}
	// A camera photo with orientation 6 is displayed with a 90-degree rotation.
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	segment := append([]byte("Exif\x00\x00"), tiff...)
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(segment)+2))
	data := append([]byte{}, encoded.Bytes()[:2]...)
	data = append(data, 0xff, 0xe1)
	data = append(data, length...)
	data = append(data, segment...)
	data = append(data, encoded.Bytes()[2:]...)
	compressed, w, h, err := compressImage(data)
	if err != nil {
		t.Fatal(err)
	}
	if w != 20 || h != 40 {
		t.Fatalf("orientation not preserved: %dx%d", w, h)
	}
	if jpegOrientation(compressed) != 1 {
		t.Fatal("EXIF metadata not removed")
	}
	for _, bad := range [][]byte{nil, {0xff, 0xd8}, {0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff}} {
		if jpegOrientation(bad) != 1 {
			t.Fatal("malformed metadata must be ignored")
		}
	}
	grid := image.NewRGBA(image.Rect(0, 0, 2, 3))
	grid.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	rotated := orientImage(grid, 6)
	if rotated.Bounds().Dx() != 3 || rotated.Bounds().Dy() != 2 || rotated.RGBAAt(2, 0).R != 255 {
		t.Fatal("incorrect rotation")
	}
}
