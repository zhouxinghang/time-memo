package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"timememo/internal/store"
)

// Each run uses its own schema; the supplied database is never truncated.
func TestIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.DB.Close()
	schema := "tm_test_" + randomID()
	if _, err = admin.DB.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.DB.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal("migration not idempotent:", err)
	}
	alice, err := s.CreateUser(ctx, "alice", "林间", "a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.CreateUser(ctx, "bob", "知秋", "another-test-password")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	h, err := New(s, Config{UploadDir: dir, Assets: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>TimeMemo")}}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	request := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		}
		req, _ := http.NewRequest(method, srv.URL+path, reader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, res.StatusCode, status, raw)
		}
		result := map[string]any{}
		_ = json.Unmarshal(raw, &result)
		return result
	}
	for _, p := range []string{"/api/me", "/api/posts", "/api/posts/1", "/api/posts/1/comments", "/api/stats", "/media/" + strings.Repeat("a", 32) + ".jpg"} {
		request("GET", p, nil, 401)
	}
	request("POST", "/api/register", map[string]string{}, 404)
	request("POST", "/api/login", map[string]any{"username": "alice", "password": "incorrect"}, 401)
	request("POST", "/api/login", map[string]any{"username": "alice", "password": "a-long-test-password", "remember": true}, 200)
	me := request("GET", "/api/me", nil, 200)
	if me["username"] != "alice" {
		t.Fatal("wrong current user")
	}
	parsed, _ := url.Parse(srv.URL)
	cookies := jar.Cookies(parsed)
	if len(cookies) != 1 || len(cookies[0].Value) != 64 {
		t.Fatal("missing opaque session token")
	}
	var storedToken string
	if err = s.DB.QueryRow(ctx, `SELECT token_hash FROM sessions WHERE user_id=$1`, alice.ID).Scan(&storedToken); err != nil {
		t.Fatal(err)
	}
	if storedToken == cookies[0].Value {
		t.Fatal("plaintext session token stored")
	}
	// Origin checks must protect even valid authenticated cookies.
	badOrigin, _ := http.NewRequest("POST", srv.URL+"/api/posts", strings.NewReader(`{"title":"CSRF"}`))
	badOrigin.Header.Set("Content-Type", "application/json")
	badOrigin.Header.Set("Origin", "https://attacker.invalid")
	res, err := client.Do(badOrigin)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross-origin mutation not rejected")
	}
	// Real multipart uploads must store only re-encoded JPEGs, including for HEIC.
	var raw bytes.Buffer
	src := image.NewRGBA(image.Rect(0, 0, 2200, 1100))
	src.Set(10, 10, color.White)
	png.Encode(&raw, src)
	heicData, err := os.ReadFile("testdata/quadrants.heic")
	if err != nil {
		t.Fatal(err)
	}
	var mediaURL string
	for i, input := range []struct {
		name string
		data []byte
	}{{"photo.png", raw.Bytes()}, {"photo.HEIC", heicData}} {
		var form bytes.Buffer
		mw := multipart.NewWriter(&form)
		part, err := mw.CreateFormFile("image", input.name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(input.data)
		mw.Close()
		upload, _ := http.NewRequest("POST", srv.URL+"/api/uploads", &form)
		upload.Header.Set("Content-Type", mw.FormDataContentType())
		res, err = client.Do(upload)
		if err != nil {
			t.Fatal(err)
		}
		var img map[string]any
		json.NewDecoder(res.Body).Decode(&img)
		res.Body.Close()
		if res.StatusCode != 201 {
			t.Fatalf("%s: %v", input.name, img)
		}
		mediaURL = img["url"].(string)
		if img["width"] != float64(1920) || img["height"] != float64(960) || img["originalSize"] != float64(len(input.data)) {
			t.Fatalf("%s: incorrect image metadata: %v", input.name, img)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != i+1 {
			t.Fatal("raw upload or temporary files left on disk:", err)
		}
		disk, err := os.ReadFile(filepath.Join(dir, strings.TrimPrefix(mediaURL, "/media/")))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(disk))
		if err != nil || cfg.Width != 1920 || cfg.Height != 960 || img["size"] != float64(len(disk)) {
			t.Fatalf("%s: stored file must be a resized JPEG: %v", input.name, err)
		}
	}
	// Uploaded previews remain owner-only until publication.
	otherJar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: otherJar}
	otherReq, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"username":"bob","password":"another-test-password"}`))
	otherReq.Header.Set("Content-Type", "application/json")
	res, err = other.Do(otherReq)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = other.Get(srv.URL + mediaURL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal("unpublished upload leaked")
	}
	p := request("POST", "/api/posts", map[string]any{"title": "秋日手记", "body": "留住今天的光。\n\n![秋日](" + mediaURL + ")", "category": "daily", "tags": []string{"秋天", "生活"}}, 201)
	id := int64(p["id"].(float64))
	path := fmt.Sprintf("/api/posts/%d", id)
	request("GET", path, nil, 200)
	request("POST", path+"/comments", map[string]string{"body": "这段时光真好。"}, 201)
	commentList := request("GET", path+"/comments", nil, 200)
	if commentList["total"] != float64(1) {
		t.Fatal(commentList)
	}
	request("POST", path+"/bookmark", map[string]bool{"saved": true}, 200)
	request("POST", path+"/bookmark", map[string]bool{"saved": true}, 200)
	saved := request("GET", "/api/posts?bookmarked=true", nil, 200)
	if saved["total"] != float64(1) {
		t.Fatal("bookmark not idempotent")
	}
	for _, query := range []string{"?q=秋日", "?tag=秋天", "?category=daily", "?mine=true", "?sort=popular"} {
		result := request("GET", "/api/posts"+query, nil, 200)
		if result["total"] != float64(1) {
			t.Fatal(query, result)
		}
	}
	// Upgrade a populated database with the original five-topic constraint.
	if _, err = s.DB.Exec(ctx, `ALTER TABLE posts DROP CONSTRAINT posts_category_check;
		ALTER TABLE posts ADD CONSTRAINT posts_category_check CHECK (category IN ('daily','thoughts','travel','culture','tech'));
		DELETE FROM schema_migrations WHERE version=2`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.Migrate(ctx); err != nil {
			t.Fatal("archive topic migration failed:", err)
		}
	}
	if existing := request("GET", path, nil, 200); existing["category"] != "daily" || existing["title"] != "秋日手记" {
		t.Fatal("migration changed existing content:", existing)
	}
	archived := request("POST", "/api/posts", map[string]any{"title": "旧日来信", "body": "把曾经的文字，安放在时光旧匣。", "category": "archive"}, 201)
	archiveID := int64(archived["id"].(float64))
	if detail := request("GET", fmt.Sprintf("/api/posts/%d", archiveID), nil, 200); detail["category"] != "archive" {
		t.Fatal("archive topic was not saved:", detail)
	}
	archiveList := request("GET", "/api/posts?category=archive", nil, 200)
	if archiveList["total"] != float64(1) || archiveList["posts"].([]any)[0].(map[string]any)["id"] != float64(archiveID) {
		t.Fatal("archive filter returned wrong posts:", archiveList)
	}
	if dailyList := request("GET", "/api/posts?category=daily", nil, 200); dailyList["total"] != float64(1) {
		t.Fatal("archive post leaked into another topic:", dailyList)
	}
	request("GET", "/api/posts?q=absent", nil, 200)
	request("GET", "/api/posts?category=private", nil, 400)
	request("POST", "/api/posts", map[string]any{"title": "", "body": "a", "category": "daily"}, 400)
	// Every logged-in member can read every published post and its images.
	res, err = other.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("member cannot read post")
	}
	res, err = other.Get(srv.URL + mediaURL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
		t.Fatal("published media access or cache protection failed")
	}
	res.Body.Close()
	for _, method := range []string{"PATCH", "PUT", "DELETE"} {
		request(method, path, map[string]string{"title": "changed"}, 404)
		request(method, path+"/comments", map[string]string{"body": "changed"}, 404)
	}
	for _, table := range []string{"posts", "comments", "post_images", "comment_images", "uploads"} {
		for _, sql := range []string{"UPDATE " + table + " SET " + map[string]string{"posts": "title=title", "comments": "body=body", "post_images": "post_id=post_id", "comment_images": "comment_id=comment_id", "uploads": "width=width"}[table], "DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			if _, err = s.DB.Exec(ctx, sql); err == nil {
				t.Fatalf("immutability bypass: %s", sql)
			}
		}
	}
	stats := request("GET", "/api/stats", nil, 200)
	categoryStats := stats["categories"].(map[string]any)
	if stats["members"] != float64(2) || stats["comments"] != float64(1) || stats["posts"] != float64(2) || categoryStats["archive"] != float64(1) {
		t.Fatal(stats)
	}
	request("POST", "/api/logout", nil, 200)
	request("GET", "/api/posts", nil, 401)
	res, err = client.Get(srv.URL + mediaURL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("image visible after logout")
	}
	// An expired token must also fail, without relying on browser cookie expiry.
	_, err = s.DB.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES ($1,$2,now()-interval '1 minute')`, tokenHash(strings.Repeat("b", 64)), bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(parsed, []*http.Cookie{{Name: cookieName, Value: strings.Repeat("b", 64), Path: "/"}})
	request("GET", "/api/me", nil, 401)
}
