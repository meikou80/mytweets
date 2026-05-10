package main

import (
	"context"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "tweets.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
	})
	return store
}

func TestSearchPostsEmpty(t *testing.T) {
	store := testStore(t)
	posts, err := store.SearchPosts(context.Background(), "anything", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatalf("len(posts) = %d, want 0", len(posts))
	}
}

func TestSavePostDedupesAndKeepsSources(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	post := StoredPost{
		Post: Post{
			ID:        "101",
			AuthorID:  "1",
			Username:  "alice",
			Text:      "Go and SQLite",
			CreatedAt: "2026-05-09T00:00:00Z",
			Lang:      "en",
		},
		RawJSON: `{"id":"101"}`,
	}

	first, err := store.SavePost(ctx, post, sourceUser, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Inserted || first.Updated {
		t.Fatalf("first save = %+v, want inserted only", first)
	}
	second, err := store.SavePost(ctx, post, sourceKeyword, "Go")
	if err != nil {
		t.Fatal(err)
	}
	if second.Inserted || !second.Updated {
		t.Fatalf("second save = %+v, want updated only", second)
	}

	all, err := store.SearchPosts(ctx, "Go", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("all len = %d, want 1", len(all))
	}
	users, err := store.SearchPosts(ctx, "Go", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("user len = %d, want 1", len(users))
	}
	keywords, err := store.SearchPosts(ctx, "Go", "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if len(keywords) != 1 {
		t.Fatalf("keyword len = %d, want 1", len(keywords))
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)

	saved, err := store.SaveConfig(ctx, AppConfig{
		Users:              []string{"@Alice", "alice"},
		Keywords:           []string{" Go ", "Go"},
		KeywordSearchMode:  KeywordSearchSeparate,
		SearchEndpoint:     SearchEndpointRecent,
		ExcludeReposts:     true,
		ExcludeReplies:     false,
		MaxPagesPerRefresh: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(saved.Users), 1; got != want {
		t.Fatalf("saved users = %d, want %d", got, want)
	}

	cfg, err := store.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Users[0], "Alice"; got != want {
		t.Fatalf("user = %q, want %q", got, want)
	}
	if got, want := cfg.Keywords[0], "Go"; got != want {
		t.Fatalf("keyword = %q, want %q", got, want)
	}
	if cfg.ExcludeReplies {
		t.Fatal("exclude replies should keep explicit false")
	}
}
