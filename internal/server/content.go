package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"timememo/internal/store"
)

var categories = map[string]bool{"daily": true, "thoughts": true, "travel": true, "culture": true, "tech": true}
var mediaPattern = regexp.MustCompile(`/media/([a-f0-9]{32}\.jpg)`)

type Post struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	Body         string     `json:"body,omitempty"`
	Excerpt      string     `json:"excerpt"`
	Category     string     `json:"category"`
	Tags         []string   `json:"tags"`
	CreatedAt    time.Time  `json:"createdAt"`
	Author       store.User `json:"author"`
	CommentCount int        `json:"commentCount"`
	Bookmarked   bool       `json:"bookmarked"`
	Image        string     `json:"image"`
	ReadMinutes  int        `json:"readMinutes"`
}

const postFields = `p.id,p.title,p.body,p.category,p.tags,p.created_at,u.id,u.username,u.display_name,u.created_at,
 (SELECT count(*) FROM comments c WHERE c.post_id=p.id),
 EXISTS(SELECT 1 FROM bookmarks b WHERE b.post_id=p.id AND b.user_id=$1),
 COALESCE((SELECT '/media/'||pi.upload_id FROM post_images pi JOIN uploads up ON up.id=pi.upload_id WHERE pi.post_id=p.id ORDER BY up.created_at,up.id LIMIT 1),'')`

func scanPost(row interface{ Scan(...any) error }) (Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.Title, &p.Body, &p.Category, &p.Tags, &p.CreatedAt, &p.Author.ID, &p.Author.Username, &p.Author.DisplayName, &p.Author.CreatedAt, &p.CommentCount, &p.Bookmarked, &p.Image)
	p.ReadMinutes = (utf8.RuneCountInString(p.Body) + 399) / 400
	p.Excerpt = excerpt(p.Body)
	return p, err
}

var markdownImages = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)

func excerpt(s string) string {
	s = markdownImages.ReplaceAllString(s, "")
	s = strings.NewReplacer("#", "", "*", "", "`", "", ">", "", "\n", " ").Replace(s)
	rs := []rune(strings.TrimSpace(s))
	if len(rs) > 120 {
		return string(rs[:120]) + "…"
	}
	return string(rs)
}

func (a *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 100 {
		fail(w, 400, "搜索词最多 100 个字符")
		return
	}
	category := r.URL.Query().Get("category")
	if category != "" && !categories[category] {
		fail(w, 400, "未知话题")
		return
	}
	tag := r.URL.Query().Get("tag")
	if utf8.RuneCountInString(tag) > 20 {
		fail(w, 400, "标签过长")
		return
	}
	page := pageNumber(r)
	uid := currentUser(r).ID
	filter := ` FROM posts p JOIN users u ON u.id=p.author_id WHERE ($2='' OR p.category=$2) AND ($3='' OR strpos(lower(p.title||' '||p.body),lower($3))>0) AND (NOT $4 OR p.author_id=$1) AND (NOT $5 OR EXISTS(SELECT 1 FROM bookmarks b WHERE b.post_id=p.id AND b.user_id=$1)) AND ($6='' OR $6=ANY(p.tags))`
	args := []any{uid, category, q, r.URL.Query().Get("mine") == "true", r.URL.Query().Get("bookmarked") == "true", tag}
	var total int
	if err := a.store.DB.QueryRow(r.Context(), "SELECT count(*)"+filter, args...).Scan(&total); err != nil {
		dbError(w, err)
		return
	}
	order := `p.created_at DESC,p.id DESC`
	if r.URL.Query().Get("sort") == "popular" {
		order = `(SELECT count(*) FROM comments c WHERE c.post_id=p.id) DESC,p.created_at DESC,p.id DESC`
	}
	rows, err := a.store.DB.Query(r.Context(), "SELECT "+postFields+filter+" ORDER BY "+order+" LIMIT 12 OFFSET $7", append(args, (page-1)*12)...)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	posts := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		p.Body = ""
		posts = append(posts, p)
	}
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	write(w, 200, map[string]any{"posts": posts, "total": total, "page": page, "pageSize": 12})
}

func (a *Server) getPost(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	p, err := scanPost(a.store.DB.QueryRow(r.Context(), "SELECT "+postFields+" FROM posts p JOIN users u ON u.id=p.author_id WHERE p.id=$2", currentUser(r).ID, id))
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, p)
}

type postInput struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

func validatePost(in *postInput) error {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > 120 {
		return fmt.Errorf("标题须为 1–120 个字符")
	}
	if n := utf8.RuneCountInString(in.Body); n < 1 || n > 50000 {
		return fmt.Errorf("正文须为 1–50000 个字符")
	}
	if !categories[in.Category] {
		return fmt.Errorf("请选择一个话题")
	}
	if len(in.Tags) > 5 {
		return fmt.Errorf("最多添加 5 个标签")
	}
	seen := map[string]bool{}
	tags := []string{}
	for _, t := range in.Tags {
		t = strings.TrimSpace(t)
		if n := utf8.RuneCountInString(t); n < 1 || n > 20 {
			return fmt.Errorf("每个标签须为 1–20 个字符")
		}
		if !seen[t] {
			tags = append(tags, t)
			seen[t] = true
		}
	}
	in.Tags = tags
	return nil
}
func attachImages(ctx context.Context, tx pgx.Tx, body string, uid, id int64, comment bool) error {
	seen := map[string]bool{}
	for _, match := range mediaPattern.FindAllStringSubmatch(body, -1) {
		seen[match[1]] = true
	}
	if len(seen) > 20 {
		return fmt.Errorf("最多插入 20 张图片")
	}
	for name := range seen {
		var owner int64
		if err := tx.QueryRow(ctx, `SELECT owner_id FROM uploads WHERE id=$1`, name).Scan(&owner); err != nil || owner != uid {
			return fmt.Errorf("图片须由当前用户上传，请重新插入")
		}
		query := `INSERT INTO post_images (post_id,upload_id) VALUES ($1,$2)`
		if comment {
			query = `INSERT INTO comment_images (comment_id,upload_id) VALUES ($1,$2)`
		}
		if _, err := tx.Exec(ctx, query, id, name); err != nil {
			return err
		}
	}
	return nil
}
func (a *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var in postInput
	if !decode(w, r, &in) {
		return
	}
	if err := validatePost(&in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	tx, err := a.store.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id int64
	uid := currentUser(r).ID
	if err = tx.QueryRow(r.Context(), `INSERT INTO posts(author_id,title,body,category,tags) VALUES ($1,$2,$3,$4,$5) RETURNING id`, uid, in.Title, in.Body, in.Category, in.Tags).Scan(&id); err != nil {
		dbError(w, err)
		return
	}
	if err = attachImages(r.Context(), tx, in.Body, uid, id, false); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	write(w, 201, map[string]int64{"id": id})
}

type Comment struct {
	ID        int64      `json:"id"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"createdAt"`
	Author    store.User `json:"author"`
}

func (a *Server) listComments(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var exists bool
	if err := a.store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM posts WHERE id=$1)`, id).Scan(&exists); err != nil {
		dbError(w, err)
		return
	}
	if !exists {
		fail(w, 404, "文章不存在")
		return
	}
	var total int
	if err := a.store.DB.QueryRow(r.Context(), `SELECT count(*) FROM comments WHERE post_id=$1`, id).Scan(&total); err != nil {
		dbError(w, err)
		return
	}
	page := pageNumber(r)
	rows, err := a.store.DB.Query(r.Context(), `SELECT c.id,c.body,c.created_at,u.id,u.username,u.display_name,u.created_at FROM comments c JOIN users u ON u.id=c.author_id WHERE c.post_id=$1 ORDER BY c.created_at,c.id LIMIT 30 OFFSET $2`, id, (page-1)*30)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	comments := []Comment{}
	for rows.Next() {
		var c Comment
		if err = rows.Scan(&c.ID, &c.Body, &c.CreatedAt, &c.Author.ID, &c.Author.Username, &c.Author.DisplayName, &c.Author.CreatedAt); err != nil {
			dbError(w, err)
			return
		}
		comments = append(comments, c)
	}
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	write(w, 200, map[string]any{"comments": comments, "total": total, "page": page, "pageSize": 30})
}
func (a *Server) createComment(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if n := utf8.RuneCountInString(in.Body); n < 1 || n > 5000 {
		fail(w, 400, "评论须为 1–5000 个字符")
		return
	}
	tx, err := a.store.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var cid int64
	if err = tx.QueryRow(r.Context(), `INSERT INTO comments(post_id,author_id,body) SELECT id,$2,$3 FROM posts WHERE id=$1 RETURNING id`, id, currentUser(r).ID, in.Body).Scan(&cid); err != nil {
		dbError(w, err)
		return
	}
	if err = attachImages(r.Context(), tx, in.Body, currentUser(r).ID, cid, true); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	write(w, 201, map[string]int64{"id": cid})
}

func (a *Server) bookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var in struct {
		Saved bool `json:"saved"`
	}
	if !decode(w, r, &in) {
		return
	}
	var exists bool
	if err := a.store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM posts WHERE id=$1)`, id).Scan(&exists); err != nil {
		dbError(w, err)
		return
	}
	if !exists {
		fail(w, 404, "文章不存在")
		return
	}
	var err error
	if in.Saved {
		_, err = a.store.DB.Exec(r.Context(), `INSERT INTO bookmarks(user_id,post_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, currentUser(r).ID, id)
	} else {
		_, err = a.store.DB.Exec(r.Context(), `DELETE FROM bookmarks WHERE user_id=$1 AND post_id=$2`, currentUser(r).ID, id)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, map[string]bool{"saved": in.Saved})
}

func (a *Server) stats(w http.ResponseWriter, r *http.Request) {
	var posts, comments, members, mine, saved int
	if err := a.store.DB.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM posts),(SELECT count(*) FROM comments),(SELECT count(*) FROM users),(SELECT count(*) FROM posts WHERE author_id=$1),(SELECT count(*) FROM bookmarks WHERE user_id=$1)`, currentUser(r).ID).Scan(&posts, &comments, &members, &mine, &saved); err != nil {
		dbError(w, err)
		return
	}
	counts := map[string]int{}
	rows, err := a.store.DB.Query(r.Context(), `SELECT category,count(*) FROM posts GROUP BY category`)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var c string
		var n int
		if err = rows.Scan(&c, &n); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		counts[c] = n
	}
	rows.Close()
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	type tagCount struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	tags := []tagCount{}
	rows, err = a.store.DB.Query(r.Context(), `SELECT tag,count(*) AS n FROM posts CROSS JOIN LATERAL unnest(tags) AS tag GROUP BY tag ORDER BY n DESC,tag LIMIT 10`)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var t tagCount
		if err = rows.Scan(&t.Tag, &t.Count); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		tags = append(tags, t)
	}
	rows.Close()
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	write(w, 200, map[string]any{"posts": posts, "comments": comments, "members": members, "mine": mine, "saved": saved, "categories": counts, "tags": tags})
}
