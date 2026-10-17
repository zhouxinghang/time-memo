package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"timememo/internal/store"
)

const cookieName = "timememo_session"

func tokenHash(raw string) string { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func randomID() string            { return hex.EncodeToString(randomBytes(16)) }
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func (a *Server) initAuth() { a.dummyHash, _ = bcrypt.GenerateFromPassword(randomBytes(32), 12) }

func (a *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !a.limiter.allow(ip) {
		w.Header().Set("Retry-After", "900")
		fail(w, 429, "尝试次数过多，请 15 分钟后再试")
		return
	}
	if len(in.Username) > 32 || len(in.Password) > 72 {
		fail(w, 401, "用户名或密码不正确")
		return
	}
	var u store.User
	var hash string
	err := a.store.DB.QueryRow(r.Context(), `SELECT id,username,display_name,created_at,password_hash FROM users WHERE username=$1`, strings.ToLower(strings.TrimSpace(in.Username))).Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &hash)
	compare := []byte(hash)
	if err != nil {
		compare = a.dummyHash
	}
	passwordErr := bcrypt.CompareHashAndPassword(compare, []byte(in.Password))
	if err != nil || passwordErr != nil {
		fail(w, 401, "用户名或密码不正确")
		return
	}
	token := hex.EncodeToString(randomBytes(32))
	age := 24 * time.Hour
	if in.Remember {
		age = 30 * 24 * time.Hour
	}
	tx, err := a.store.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if c, e := r.Cookie(cookieName); e == nil {
		if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			dbError(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		dbError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO sessions (token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, tokenHash(token), u.ID, time.Now().Add(age)); err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: a.config.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: int(age.Seconds()), Expires: time.Now().Add(age)})
	write(w, 200, u)
}

func (a *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie(cookieName)
		if e != nil || len(c.Value) != 64 {
			fail(w, 401, "请先登录")
			return
		}
		var u store.User
		err := a.store.DB.QueryRow(r.Context(), `SELECT u.id,u.username,u.display_name,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash(c.Value)).Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				dbError(w, err)
				return
			}
			fail(w, 401, "登录已过期，请重新登录")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}

func (a *Server) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(cookieName)
	if _, err := a.store.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
		dbError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.config.SecureCookie, SameSite: http.SameSiteLaxMode})
	write(w, 200, map[string]bool{"ok": true})
}
