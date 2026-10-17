package server

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxUpload = 12 << 20

var imageName = regexp.MustCompile(`^[a-f0-9]{32}\.jpg$`)

// compressImage decodes in memory, removes metadata and flattens transparency.
// Only the resized, re-encoded JPEG is ever written to disk.
func compressImage(data []byte) ([]byte, int, int, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("图片无法读取，请使用 JPEG、PNG 或 WebP")
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return nil, 0, 0, fmt.Errorf("仅支持 JPEG、PNG 和 WebP")
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 25_000_000 {
		return nil, 0, 0, fmt.Errorf("图片最多 2500 万像素")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("图片已损坏")
	}
	w, h := cfg.Width, cfg.Height
	if w > 1920 || h > 1920 {
		if w >= h {
			h = max(1, h*1920/w)
			w = 1920
		} else {
			w = max(1, w*1920/h)
			h = 1920
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	stddraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), stddraw.Over, nil)
	if format == "jpeg" {
		dst = orientImage(dst, jpegOrientation(data))
		w, h = dst.Bounds().Dx(), dst.Bounds().Dy()
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), w, h, nil
}

func (a *Server) upload(w http.ResponseWriter, r *http.Request) {
	select {
	case a.imageSlots <- struct{}{}:
		defer func() { <-a.imageSlots }()
	default:
		fail(w, 429, "图片正在处理中，请稍后重试")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, "请上传图片文件")
		return
	}
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "image" {
		fail(w, 400, "请选择图片")
		return
	}
	defer part.Close()
	data, err := io.ReadAll(io.LimitReader(part, maxUpload+1))
	if err != nil || len(data) > maxUpload {
		fail(w, 413, "图片不能超过 12 MB")
		return
	}
	compressed, width, height, err := compressImage(data)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	name := randomID() + ".jpg"
	tmp, err := os.CreateTemp(a.config.UploadDir, ".compressed-*.jpg")
	if err != nil {
		fail(w, 500, "图片保存失败")
		return
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(compressed); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil || closeErr != nil {
		fail(w, 500, "图片保存失败")
		return
	}
	full := filepath.Join(a.config.UploadDir, name)
	if err = os.Rename(tmp.Name(), full); err != nil {
		fail(w, 500, "图片保存失败")
		return
	}
	_, err = a.store.DB.Exec(r.Context(), `INSERT INTO uploads(id,owner_id,width,height,size) VALUES ($1,$2,$3,$4,$5)`, name, currentUser(r).ID, width, height, len(compressed))
	if err != nil {
		os.Remove(full)
		dbError(w, err)
		return
	}
	write(w, 201, map[string]any{"url": "/media/" + name, "width": width, "height": height, "size": len(compressed), "originalSize": len(data)})
}

func (a *Server) media(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !imageName.MatchString(name) {
		fail(w, 404, "图片不存在")
		return
	}
	var visible bool
	err := a.store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM uploads up WHERE up.id=$1 AND (up.owner_id=$2 OR EXISTS(SELECT 1 FROM post_images pi WHERE pi.upload_id=up.id) OR EXISTS(SELECT 1 FROM comment_images ci WHERE ci.upload_id=up.id)))`, name, currentUser(r).ID).Scan(&visible)
	if err != nil {
		dbError(w, err)
		return
	}
	if !visible {
		fail(w, 404, "图片不存在")
		return
	}
	f, err := os.Open(filepath.Join(a.config.UploadDir, name))
	if err != nil {
		fail(w, 404, "图片不存在")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		fail(w, 404, "图片不存在")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "inline; filename="+name)
	http.ServeContent(w, r, name, info.ModTime(), f)
}
