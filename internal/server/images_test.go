package server

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
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

func TestCompressHEIC(t *testing.T) {
	original, err := os.ReadFile("testdata/quadrants.heic")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		brand    string
		rotation byte
		width    int
		height   int
		topLeft  color.RGBA
	}{
		{"HEIC", "heic", 0, 1920, 960, color.RGBA{R: 240, G: 40, B: 30, A: 255}},
		{"HEIF", "mif1", 0, 1920, 960, color.RGBA{R: 240, G: 40, B: 30, A: 255}},
		{"rotated HEIC", "heic", 1, 960, 1920, color.RGBA{R: 30, G: 220, B: 40, A: 255}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := bytes.Clone(original)
			copy(raw[8:12], tt.brand)
			rotation := bytes.Index(raw, []byte("irot"))
			if rotation < 0 {
				t.Fatal("fixture is missing the rotation property")
			}
			raw[rotation+4] = tt.rotation
			data, w, h, err := compressImage(raw)
			if err != nil {
				t.Fatal(err)
			}
			if w != tt.width || h != tt.height {
				t.Fatalf("got %dx%d, want %dx%d", w, h, tt.width, tt.height)
			}
			img, err := jpeg.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal("HEIC must be re-encoded as JPEG:", err)
			}
			if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
				t.Fatal("JPEG dimensions do not match upload metadata")
			}
			pixel := color.RGBAModel.Convert(img.At(w/4, h/4)).(color.RGBA)
			for i, got := range []uint8{pixel.R, pixel.G, pixel.B} {
				want := []uint8{tt.topLeft.R, tt.topLeft.G, tt.topLeft.B}[i]
				if delta := int(got) - int(want); delta < -30 || delta > 30 {
					t.Fatalf("incorrect colors or orientation: got %v, want approximately %v", pixel, tt.topLeft)
				}
			}
		})
	}
}

func TestCompressHEICRejectsInvalidImages(t *testing.T) {
	raw, err := os.ReadFile("testdata/quadrants.heic")
	if err != nil {
		t.Fatal(err)
	}
	oversized := bytes.Clone(raw)
	ispe := bytes.LastIndex(oversized, []byte("ispe"))
	if ispe < 0 {
		t.Fatal("fixture is missing the image size property")
	}
	binary.BigEndian.PutUint32(oversized[ispe+8:ispe+12], 6000)
	binary.BigEndian.PutUint32(oversized[ispe+12:ispe+16], 5000)
	if _, _, _, err := compressImage(oversized); err == nil || !strings.Contains(err.Error(), "2500 万像素") {
		t.Fatalf("expected pixel limit rejection, got %v", err)
	}
	// HEIF dimensions are uint32; their product can overflow int64.
	binary.BigEndian.PutUint32(oversized[ispe+8:ispe+12], ^uint32(0))
	binary.BigEndian.PutUint32(oversized[ispe+12:ispe+16], ^uint32(0))
	if _, _, _, err := compressImage(oversized); err == nil {
		t.Fatal("overflowing image dimensions accepted")
	}
	broken := bytes.Clone(raw)
	mdat := bytes.Index(broken, []byte("mdat"))
	if mdat < 0 {
		t.Fatal("fixture is missing the image data box")
	}
	broken = broken[:mdat+12] // Keep the valid header but truncate the HEVC image data.
	for _, data := range [][]byte{raw[:12], broken} {
		if _, _, _, err := compressImage(data); err == nil {
			t.Fatal("malformed HEIC accepted")
		}
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
