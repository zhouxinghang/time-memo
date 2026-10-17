package store

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

type User struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Store struct{ DB *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	if url == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("invalid DATABASE_URL")
	}
	cfg.MaxConns = 12
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return &Store{DB: db}, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7469636)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return tx.Commit(ctx)
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

func (s *Store) CreateUser(ctx context.Context, username, name, password string) (User, error) {
	u := User{Username: strings.ToLower(strings.TrimSpace(username)), DisplayName: strings.TrimSpace(name)}
	if !usernamePattern.MatchString(u.Username) {
		return u, fmt.Errorf("用户名须为 3–32 位小写字母、数字、下划线或短横线")
	}
	if utf8.RuneCountInString(u.DisplayName) < 1 || utf8.RuneCountInString(u.DisplayName) > 40 {
		return u, fmt.Errorf("昵称须为 1–40 个字符")
	}
	if len(password) < 12 || len(password) > 72 {
		return u, fmt.Errorf("密码须为 12–72 字节")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return u, err
	}
	err = s.DB.QueryRow(ctx, `INSERT INTO users (username, display_name, password_hash) VALUES ($1,$2,$3) RETURNING id, created_at`, u.Username, u.DisplayName, string(hash)).Scan(&u.ID, &u.CreatedAt)
	return u, err
}
