package dailynote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"timememo/internal/store"
)

const testUUID = "12345678-1234-1234-1234-123456789abc"
const testText = "把每一次真诚的表达，安放在岁月的长卷里。"

func TestCalendarDay(t *testing.T) {
	for _, tc := range []struct{ time, date string }{{"2026-10-17T15:59:59Z", "2026-10-17"}, {"2026-10-17T16:00:00Z", "2026-10-18"}, {"2026-12-31T16:00:00Z", "2027-01-01"}} {
		now, err := time.Parse(time.RFC3339, tc.time)
		if err != nil {
			t.Fatal(err)
		}
		day := calendarDay(now)
		if day.Format("2006-01-02") != tc.date || day.Hour() != 0 {
			t.Fatalf("%s => %s", tc.time, day)
		}
		_, offset := day.Zone()
		if offset != 8*60*60 {
			t.Fatal("not Beijing time")
		}
	}
}

func TestLocalNotes(t *testing.T) {
	seen := map[string]bool{}
	if len(localNotes) <= recentDays {
		t.Fatal("not enough local notes for deduplication")
	}
	for _, text := range localNotes {
		if n := utf8.RuneCountInString(text); n < 10 || n > 40 {
			t.Fatalf("invalid local note length %d: %s", n, text)
		}
		if seen[contentHash(text)] {
			t.Fatal("duplicate local note")
		}
		seen[contentHash(text)] = true
	}
	day := calendarDay(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	history := []string{}
	for i := 0; i < 400; i++ {
		recent := map[string]bool{}
		for _, h := range history {
			recent[h] = true
		}
		c := fallback(day.AddDate(0, 0, i), recent)
		hash := contentHash(c.content)
		if recent[hash] || c.source != "时光记" || c.provider != "builtin" {
			t.Fatal("invalid or repeated fallback")
		}
		history = append(history, hash)
		if len(history) > recentDays {
			history = history[1:]
		}
	}
}

func TestFetch(t *testing.T) {
	valid := map[string]any{"hitokoto": testText, "uuid": testUUID, "type": "d", "from": "随想", "from_who": nil}
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"valid", 200, jsonText(valid), false},
		{"rate limited", 429, "{}", true},
		{"malformed", 200, "not json", true},
		{"oversized", 200, strings.Repeat("x", (16<<10)+1), true},
		{"short sentence", 200, jsonText(with(valid, "hitokoto", "太短")), true},
		{"wrong category", 200, jsonText(with(valid, "type", "a")), true},
		{"invalid uuid", 200, jsonText(with(valid, "uuid", "https://invalid.example")), true},
		{"invalid metadata", 200, jsonText(with(valid, "from", "出处\n换行")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected upstream request")
				}
				if got := r.URL.Query()["c"]; strings.Join(got, ",") != "d,i,k" {
					t.Error("missing category filter")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			s, _ := New(nil, "hitokoto")
			s.endpoint = upstream.URL + "/?c=d&c=i&c=k&min_length=10&max_length=40"
			c, err := s.fetch(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("candidate=%+v err=%v", c, err)
			}
			if !tc.wantErr && (c.content != testText || c.author != "" || c.id != testUUID) {
				t.Fatal(c)
			}
		})
	}
	if _, err := New(nil, "unknown"); err == nil {
		t.Fatal("invalid provider accepted")
	}
}

func TestFetchTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer upstream.Close()
	s, _ := New(nil, "hitokoto")
	s.endpoint = upstream.URL
	s.client.Timeout = 20 * time.Millisecond
	start := time.Now()
	if _, err := s.fetch(context.Background()); err == nil {
		t.Fatal("provider timeout ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("request was not bounded")
	}
}

func jsonText(v any) string { data, _ := json.Marshal(v); return string(data) }
func with(base map[string]any, key string, value any) map[string]any {
	copy := map[string]any{}
	for k, v := range base {
		copy[k] = v
	}
	copy[key] = value
	return copy
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run daily note database tests")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.DB.Close)
	schema := fmt.Sprintf("tm_note_%d", time.Now().UnixNano())
	if _, err = admin.DB.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
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
	t.Cleanup(s.DB.Close)
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func fixedService(t *testing.T, db *store.Store, now time.Time, upstream string) *Service {
	t.Helper()
	s, err := New(db.DB, "hitokoto")
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	s.endpoint = upstream
	s.retryDelay = time.Millisecond
	return s
}
func seed(t *testing.T, db *store.Store, date string, c candidate) {
	t.Helper()
	_, err := db.DB.Exec(context.Background(), `INSERT INTO daily_notes(date,content,content_hash,author,source,provider,provider_id) VALUES ($1::date,$2,$3,$4,$5,$6,$7)`, date, c.content, contentHash(c.content), c.author, c.source, c.provider, c.id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetDatabase(t *testing.T) {
	now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	t.Run("concurrent selection survives restart and is immutable", func(t *testing.T) {
		db := testStore(t)
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			time.Sleep(20 * time.Millisecond)
			fmt.Fprint(w, jsonText(map[string]any{"hitokoto": testText, "uuid": testUUID, "type": "d", "from": "随想", "from_who": "时光测试"}))
		}))
		defer upstream.Close()
		a, b := fixedService(t, db, now, upstream.URL), fixedService(t, db, now, upstream.URL)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				s := a
				if i%2 == 1 {
					s = b
				}
				n, err := s.Get(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if n.Content != testText || n.Date != "2026-10-17" || n.SourceURL != "https://hitokoto.cn/?uuid="+testUUID || !n.NextRefreshAt.Equal(time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)) {
					t.Error(n)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if calls.Load() != 1 {
			t.Fatalf("upstream called %d times", calls.Load())
		}
		restarted := fixedService(t, db, now, upstream.URL)
		if n, err := restarted.Get(context.Background()); err != nil || n.Content != testText || calls.Load() != 1 {
			t.Fatal(n, err, calls.Load())
		}
		var count int
		if err := db.DB.QueryRow(context.Background(), `SELECT count(*) FROM daily_notes`).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
		for _, query := range []string{`UPDATE daily_notes SET content=content`, `DELETE FROM daily_notes`, `TRUNCATE daily_notes`} {
			if _, err := db.DB.Exec(context.Background(), query); err == nil {
				t.Fatal("daily note changed:", query)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := a.Get(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled request ignored", err)
		}
	})
	t.Run("deduplicates both text and provider id", func(t *testing.T) {
		db := testStore(t)
		old := "留下一段日常，也留住今天的阳光。"
		seed(t, db, "2026-10-16", candidate{content: old, source: "随想", provider: "hitokoto", id: testUUID})
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			text, id := testText, "22345678-1234-1234-1234-123456789abc"
			if n == 1 {
				text = old
			} else if n == 2 {
				id = testUUID
			}
			fmt.Fprint(w, jsonText(map[string]any{"hitokoto": text, "uuid": id, "type": "k", "from": "随想"}))
		}))
		defer upstream.Close()
		s := fixedService(t, db, now, upstream.URL)
		n, err := s.Get(context.Background())
		if err != nil || n.Content != testText || n.SourceURL == "" || calls.Load() != 3 {
			t.Fatal(n, err, calls.Load())
		}
	})
	t.Run("outage stays fixed until next Beijing day", func(t *testing.T) {
		db := testStore(t)
		var calls atomic.Int32
		var recovered atomic.Bool
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if !recovered.Load() {
				w.WriteHeader(429)
				return
			}
			fmt.Fprint(w, jsonText(map[string]any{"hitokoto": testText, "uuid": testUUID, "type": "i", "from": "随想"}))
		}))
		defer upstream.Close()
		s := fixedService(t, db, now, upstream.URL)
		first, err := s.Get(context.Background())
		if err != nil || first.Source != "时光记" || first.SourceURL != "" || calls.Load() != 1 {
			t.Fatal(first, err)
		}
		recovered.Store(true)
		s = fixedService(t, db, now, upstream.URL)
		same, err := s.Get(context.Background())
		if err != nil || same != first || calls.Load() != 1 {
			t.Fatal("changed during the same day", same, err)
		}
		s.now = func() time.Time { return time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC) }
		next, err := s.Get(context.Background())
		if err != nil || next.Date != "2026-10-18" || next.Content != testText || calls.Load() != 2 {
			t.Fatal(next, err, calls.Load())
		}
	})
	t.Run("repeated upstream falls back without recent duplicates", func(t *testing.T) {
		db := testStore(t)
		day := calendarDay(now)
		for i := 0; i < recentDays; i++ {
			c := candidate{content: localNotes[i], source: "时光记", provider: "builtin"}
			if i == 0 {
				c = candidate{content: testText, source: "随想", provider: "hitokoto", id: testUUID}
			}
			seed(t, db, day.AddDate(0, 0, -i-1).Format("2006-01-02"), c)
		}
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			fmt.Fprint(w, jsonText(map[string]any{"hitokoto": testText, "uuid": testUUID, "type": "d", "from": "随想"}))
		}))
		defer upstream.Close()
		s := fixedService(t, db, now, upstream.URL)
		n, err := s.Get(context.Background())
		if err != nil || n.Source != "时光记" || calls.Load() != 3 {
			t.Fatal(n, err, calls.Load())
		}
		for _, previous := range localNotes[1:30] {
			if n.Content == previous {
				t.Fatal("repeated local note")
			}
		}
	})
	t.Run("notes older than the window may be reused", func(t *testing.T) {
		db := testStore(t)
		seed(t, db, "2026-09-16", candidate{content: testText, source: "随想", provider: "hitokoto", id: testUUID})
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, jsonText(map[string]any{"hitokoto": testText, "uuid": testUUID, "type": "d", "from": "随想"}))
		}))
		defer upstream.Close()
		s := fixedService(t, db, now, upstream.URL)
		n, err := s.Get(context.Background())
		if err != nil || n.Content != testText || n.SourceURL == "" {
			t.Fatal(n, err)
		}
	})
}
