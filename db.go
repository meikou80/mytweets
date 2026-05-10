package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Post struct {
	ID        string `json:"id"`
	AuthorID  string `json:"author_id"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	URL       string `json:"url"`
	Lang      string `json:"lang"`
}

type StoredPost struct {
	Post
	RawJSON string
}

type SavePostResult struct {
	Inserted bool
	Updated  bool
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if err := s.migrateSourceKindCheck(ctx, "post_sources", `create table post_sources (
			post_id text not null,
			source_kind text not null check (source_kind in ('user', 'keyword', 'tracked_user_keyword')),
			source_key text not null,
			primary key (post_id, source_kind, source_key),
			foreign key (post_id) references posts(id) on delete cascade
		)`); err != nil {
		return err
	}
	return s.migrateSourceKindCheck(ctx, "tracking_state", `create table tracking_state (
			source_kind text not null check (source_kind in ('user', 'keyword', 'tracked_user_keyword')),
			source_key text not null,
			since_id text not null,
			primary key (source_kind, source_key)
		)`)
}

func (s *Store) migrateSourceKindCheck(ctx context.Context, table, createSQL string) error {
	var sqlText string
	err := s.db.QueryRowContext(ctx, `select sql from sqlite_master where type = 'table' and name = ?`, table).Scan(&sqlText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(sqlText, "tracked_user_keyword") {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tmp := table + "_new"
	if _, err := tx.ExecContext(ctx, `drop table if exists `+tmp); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, strings.Replace(createSQL, "create table "+table, "create table "+tmp, 1)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into `+tmp+` select * from `+table); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `drop table `+table); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `alter table `+tmp+` rename to `+table); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) init(ctx context.Context) error {
	statements := []string{
		`pragma foreign_keys = on`,
		`create table if not exists posts (
			id text primary key,
			author_id text,
			username text,
			text text not null,
			created_at text,
			url text,
			lang text,
			raw_json text,
			fetched_at text not null
		)`,
		`create table if not exists post_sources (
			post_id text not null,
			source_kind text not null check (source_kind in ('user', 'keyword', 'tracked_user_keyword')),
			source_key text not null,
			primary key (post_id, source_kind, source_key),
			foreign key (post_id) references posts(id) on delete cascade
		)`,
		`create table if not exists tracking_state (
			source_kind text not null check (source_kind in ('user', 'keyword', 'tracked_user_keyword')),
			source_key text not null,
			since_id text not null,
			primary key (source_kind, source_key)
		)`,
		`create table if not exists user_cache (
			username text primary key,
			user_id text not null,
			name text,
			resolved_at text not null
		)`,
		`create index if not exists idx_posts_created_at on posts(created_at desc)`,
		`create index if not exists idx_posts_text on posts(text)`,
		`create index if not exists idx_post_sources_source on post_sources(source_kind, source_key)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SavePost(ctx context.Context, post StoredPost, sourceKind, sourceKey string) (SavePostResult, error) {
	if post.ID == "" {
		return SavePostResult{}, errors.New("post id is empty")
	}
	if post.URL == "" && post.Username != "" {
		post.URL = "https://x.com/" + post.Username + "/status/" + post.ID
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SavePostResult{}, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `select count(1) from posts where id = ?`, post.ID).Scan(&exists); err != nil {
		return SavePostResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		insert into posts (id, author_id, username, text, created_at, url, lang, raw_json, fetched_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			author_id = excluded.author_id,
			username = excluded.username,
			text = excluded.text,
			created_at = excluded.created_at,
			url = excluded.url,
			lang = excluded.lang,
			raw_json = excluded.raw_json,
			fetched_at = excluded.fetched_at
	`, post.ID, post.AuthorID, post.Username, post.Text, post.CreatedAt, post.URL, post.Lang, post.RawJSON, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return SavePostResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		insert or ignore into post_sources (post_id, source_kind, source_key)
		values (?, ?, ?)
	`, post.ID, sourceKind, sourceKey)
	if err != nil {
		return SavePostResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return SavePostResult{}, err
	}
	return SavePostResult{Inserted: exists == 0, Updated: exists != 0}, nil
}

func (s *Store) GetSinceID(ctx context.Context, sourceKind, sourceKey string) (string, error) {
	var sinceID string
	err := s.db.QueryRowContext(ctx, `
		select since_id from tracking_state where source_kind = ? and source_key = ?
	`, sourceKind, sourceKey).Scan(&sinceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return sinceID, err
}

func (s *Store) SaveSinceID(ctx context.Context, sourceKind, sourceKey, sinceID string) error {
	if sinceID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		insert into tracking_state (source_kind, source_key, since_id)
		values (?, ?, ?)
		on conflict(source_kind, source_key) do update set since_id = excluded.since_id
	`, sourceKind, sourceKey, sinceID)
	return err
}

func (s *Store) CacheUser(ctx context.Context, user XUser) error {
	if user.Username == "" || user.ID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		insert into user_cache (username, user_id, name, resolved_at)
		values (?, ?, ?, ?)
		on conflict(username) do update set
			user_id = excluded.user_id,
			name = excluded.name,
			resolved_at = excluded.resolved_at
	`, user.Username, user.ID, user.Name, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) SearchPosts(ctx context.Context, q, source string) ([]Post, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "all"
	}
	if source != "all" && source != "user" && source != "keyword" {
		return nil, fmt.Errorf("invalid source: %s", source)
	}

	pattern := "%" + q + "%"
	var rows *sql.Rows
	var err error
	if source == "all" {
		rows, err = s.db.QueryContext(ctx, `
			select id, author_id, username, text, created_at, url, lang
			from posts
			where text like ?
			order by created_at desc
		`, pattern)
	} else {
		sourceKinds := []string{source}
		if source == sourceKeyword {
			sourceKinds = append(sourceKinds, sourceTrackedUserKeyword)
		}
		rows, err = s.db.QueryContext(ctx, `
			select distinct p.id, p.author_id, p.username, p.text, p.created_at, p.url, p.lang
			from posts p
			join post_sources ps on ps.post_id = p.id
			where p.text like ? and ps.source_kind in (`+placeholders(len(sourceKinds))+`)
			order by p.created_at desc
		`, append([]any{pattern}, stringsToAny(sourceKinds)...)...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []Post{}
	for rows.Next() {
		var post Post
		if err := rows.Scan(&post.ID, &post.AuthorID, &post.Username, &post.Text, &post.CreatedAt, &post.URL, &post.Lang); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return posts, nil
}

func placeholders(n int) string {
	if n < 1 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}
