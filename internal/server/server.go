package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"timememo/internal/store"
)

type Config struct {
	UploadDir    string
	SecureCookie bool
	Assets       fs.FS
}
type Server struct {
	store      *store.Store
	config     Config
	limiter    loginLimiter
	dummyHash  []byte
	imageSlots chan struct{}
}
type userKey struct{}

func New(s *store.Store, cfg Config) (http.Handler, error) {
	if cfg.UploadDir == "" {
		cfg.UploadDir = "./data/uploads"
	}
	if err := os.MkdirAll(cfg.UploadDir, 0700); err != nil {
		return nil, err
	}
	a := &Server{store: s, config: cfg, imageSlots: make(chan struct{}, 2)}
	a.initAuth()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.DB.Ping(ctx) != nil {
			fail(w, 503, "数据库暂时不可用")
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/login", a.login)
	mux.Handle("POST /api/logout", a.auth(http.HandlerFunc(a.logout)))
	mux.Handle("GET /api/me", a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { write(w, 200, currentUser(r)) })))
	mux.Handle("GET /api/stats", a.auth(http.HandlerFunc(a.stats)))
	mux.Handle("GET /api/posts", a.auth(http.HandlerFunc(a.listPosts)))
	mux.Handle("POST /api/posts", a.auth(http.HandlerFunc(a.createPost)))
	mux.Handle("GET /api/posts/{id}", a.auth(http.HandlerFunc(a.getPost)))
	mux.Handle("GET /api/posts/{id}/comments", a.auth(http.HandlerFunc(a.listComments)))
	mux.Handle("POST /api/posts/{id}/comments", a.auth(http.HandlerFunc(a.createComment)))
	mux.Handle("POST /api/posts/{id}/bookmark", a.auth(http.HandlerFunc(a.bookmark)))
	mux.Handle("POST /api/uploads", a.auth(http.HandlerFunc(a.upload)))
	mux.Handle("GET /media/{name}", a.auth(http.HandlerFunc(a.media)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "接口不存在") })
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "图片不存在") })
	mux.HandleFunc("/", a.frontend)
	return security(http.NewCrossOriginProtection().Handler(mux)), nil
}

func currentUser(r *http.Request) store.User { return r.Context().Value(userKey{}).(store.User) }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "请使用 JSON 格式")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "请求格式不正确或内容过长")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "请求格式不正确")
		return false
	}
	return true
}
func dbError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "内容不存在")
		return
	}
	slog.Error("database operation failed", "error", err)
	fail(w, 500, "暂时无法完成，请稍后重试")
}
func postID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 404, "文章不存在")
		return 0, false
	}
	return id, true
}
func pageNumber(r *http.Request) int {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if p < 1 {
		p = 1
	}
	if p > 100000 {
		p = 100000
	}
	return p
}

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/media/") {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		fail(w, http.StatusMethodNotAllowed, "不支持此操作")
		return
	}
	if a.config.Assets == nil {
		http.Error(w, "请先运行 npm run build", 503)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	if name == "" || name == "." {
		name = "index.html"
	}
	data, err := fs.ReadFile(a.config.Assets, name)
	if err != nil {
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		data, err = fs.ReadFile(a.config.Assets, "index.html")
	}
	if err != nil {
		http.Error(w, "请先运行 npm run build，再构建 Go 服务", 503)
		return
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(data)))
}

type loginAttempt struct {
	count int
	reset time.Time
}
type loginLimiter struct {
	sync.Mutex
	entries map[string]loginAttempt
}

func (l *loginLimiter) allow(key string) bool {
	l.Lock()
	defer l.Unlock()
	now := time.Now()
	if l.entries == nil {
		l.entries = make(map[string]loginAttempt)
	}
	for k, v := range l.entries {
		if now.After(v.reset) {
			delete(l.entries, k)
		}
	}
	v := l.entries[key]
	if v.reset.IsZero() {
		v.reset = now.Add(15 * time.Minute)
	}
	v.count++
	l.entries[key] = v
	return v.count <= 20
}
