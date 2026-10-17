// Package dailynote selects and permanently records one community-wide note per
// Beijing calendar day. External requests never contain forum content or sessions.
package dailynote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const endpoint = "https://v1.hitokoto.cn/?c=d&c=i&c=k&min_length=10&max_length=40"
const recentDays = 30

var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)
var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type Note struct {
	Date          string    `json:"date"`
	Content       string    `json:"content"`
	Author        string    `json:"author"`
	Source        string    `json:"source"`
	SourceURL     string    `json:"sourceUrl"`
	NextRefreshAt time.Time `json:"nextRefreshAt"`
}

type candidate struct{ content, author, source, provider, id string }

type Service struct {
	db         *pgxpool.Pool
	source     string
	client     *http.Client
	endpoint   string
	now        func() time.Time
	retryDelay time.Duration
	mu         sync.Mutex
	cached     Note
}

func New(db *pgxpool.Pool, source string) (*Service, error) {
	if source == "" {
		source = "hitokoto"
	}
	if source != "hitokoto" && source != "builtin" {
		return nil, fmt.Errorf("DAILY_NOTE_SOURCE must be hitokoto or builtin")
	}
	return &Service{
		db: db, source: source, endpoint: endpoint, now: time.Now,
		retryDelay: 600 * time.Millisecond,
		client: &http.Client{
			Timeout:       1500 * time.Millisecond,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func calendarDay(t time.Time) time.Time {
	t = t.In(beijing)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, beijing)
}

// Get serializes initial selection and uses a database advisory lock so even two
// service processes cannot choose different notes for the same date.
func (s *Service) Get(ctx context.Context) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Note{}, err
	}
	day := calendarDay(s.now())
	date := day.Format("2006-01-02")
	if s.cached.Date == date {
		return s.cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	note, err := readNote(s.db.QueryRow(ctx, `SELECT date::text,content,author,source,provider,provider_id FROM daily_notes WHERE date=$1::date`, date), day)
	if err == nil {
		s.cached = note
		return note, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Note{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7469637, ($1::date - DATE '2000-01-01')::integer)`, date); err != nil {
		return Note{}, err
	}
	note, err = readNote(tx.QueryRow(ctx, `SELECT date::text,content,author,source,provider,provider_id FROM daily_notes WHERE date=$1::date`, date), day)
	if err == nil {
		s.cached = note
		return note, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Note{}, err
	}
	rows, err := tx.Query(ctx, `SELECT content_hash,provider_id FROM daily_notes WHERE date >= $1::date AND date < $2::date`, day.AddDate(0, 0, -recentDays).Format("2006-01-02"), date)
	if err != nil {
		return Note{}, err
	}
	hashes, ids := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var hash, id string
		if err = rows.Scan(&hash, &id); err != nil {
			rows.Close()
			return Note{}, err
		}
		hashes[hash] = true
		if id != "" {
			ids[id] = true
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return Note{}, rows.Err()
	}
	selected := s.selectNote(ctx, day, hashes, ids)
	note, err = readNote(tx.QueryRow(ctx, `INSERT INTO daily_notes(date,content,content_hash,author,source,provider,provider_id) VALUES ($1::date,$2,$3,$4,$5,$6,$7) ON CONFLICT (date) DO NOTHING RETURNING date::text,content,author,source,provider,provider_id`, date, selected.content, contentHash(selected.content), selected.author, selected.source, selected.provider, selected.id), day)
	if errors.Is(err, pgx.ErrNoRows) {
		note, err = readNote(tx.QueryRow(ctx, `SELECT date::text,content,author,source,provider,provider_id FROM daily_notes WHERE date=$1::date`, date), day)
	}
	if err != nil {
		return Note{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Note{}, err
	}
	s.cached = note
	return note, nil
}

func readNote(row pgx.Row, day time.Time) (Note, error) {
	var note Note
	var provider, id string
	err := row.Scan(&note.Date, &note.Content, &note.Author, &note.Source, &provider, &id)
	if provider == "hitokoto" && uuidPattern.MatchString(id) {
		note.SourceURL = "https://hitokoto.cn/?uuid=" + id
	}
	note.NextRefreshAt = day.AddDate(0, 0, 1)
	return note, err
}

func contentHash(content string) string {
	hash := sha256.Sum256([]byte(strings.Join(strings.Fields(content), " ")))
	return hex.EncodeToString(hash[:])
}

func (s *Service) selectNote(ctx context.Context, day time.Time, hashes, ids map[string]bool) candidate {
	if s.source == "hitokoto" {
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(s.retryDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return fallback(day, hashes)
				case <-timer.C:
				}
			}
			c, err := s.fetch(ctx)
			if err != nil {
				slog.Warn("daily note provider unavailable; using local note", "error", err)
				break
			}
			if !hashes[contentHash(c.content)] && !ids[c.id] {
				return c
			}
		}
	}
	return fallback(day, hashes)
}

func (s *Service) fetch(ctx context.Context) (candidate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return candidate{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TimeMemo/1.0")
	res, err := s.client.Do(req)
	if err != nil {
		return candidate{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return candidate{}, fmt.Errorf("hitokoto returned HTTP %d", res.StatusCode)
	}
	const limit = 16 << 10
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return candidate{}, err
	}
	if len(data) > limit {
		return candidate{}, fmt.Errorf("hitokoto response exceeds size limit")
	}
	var in struct {
		Content string `json:"hitokoto"`
		Author  string `json:"from_who"`
		Source  string `json:"from"`
		UUID    string `json:"uuid"`
		Type    string `json:"type"`
	}
	if err = json.Unmarshal(data, &in); err != nil {
		return candidate{}, fmt.Errorf("invalid hitokoto JSON")
	}
	c := candidate{content: strings.Join(strings.Fields(in.Content), " "), author: strings.TrimSpace(in.Author), source: strings.TrimSpace(in.Source), provider: "hitokoto", id: strings.ToLower(in.UUID)}
	if c.source == "" {
		c.source = "一言"
	}
	if n := utf8.RuneCountInString(c.content); n < 10 || n > 40 {
		return candidate{}, fmt.Errorf("invalid hitokoto sentence length")
	}
	if !uuidPattern.MatchString(c.id) || (in.Type != "d" && in.Type != "i" && in.Type != "k") || utf8.RuneCountInString(c.author) > 80 || utf8.RuneCountInString(c.source) > 80 {
		return candidate{}, fmt.Errorf("invalid hitokoto metadata")
	}
	for _, v := range []string{c.content, c.author, c.source} {
		for _, r := range v {
			if unicode.IsControl(r) {
				return candidate{}, fmt.Errorf("invalid control character in hitokoto response")
			}
		}
	}
	return c, nil
}
