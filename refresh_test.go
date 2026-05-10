package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshPostsFetchesSeparateUserAndKeywordSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/2/users/by":
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "1", "name": "Alice", "username": "alice"}},
			})
		case "/2/users/1/tweets":
			json.NewEncoder(w).Encode(samplePostsResponse("101", "1", "alice"))
		case "/2/tweets/search/recent":
			json.NewEncoder(w).Encode(samplePostsResponse("202", "2", "bob"))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	store := testStore(t)
	client := NewXClient("token")
	client.BaseURL = server.URL
	cfg := AppConfig{
		Users:              []string{"alice"},
		Keywords:           []string{"Go"},
		Language:           "ja",
		ExcludeReposts:     true,
		ExcludeReplies:     true,
		MaxPagesPerRefresh: 1,
	}

	result := RefreshPosts(context.Background(), store, client, cfg)
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %+v", result.Errors)
	}
	if got, want := result.Sources, 2; got != want {
		t.Fatalf("sources = %d, want %d", got, want)
	}
	if got, want := result.Inserted, 2; got != want {
		t.Fatalf("inserted = %d, want %d", got, want)
	}
	if got, want := result.Fetched, 2; got != want {
		t.Fatalf("fetched = %d, want %d", got, want)
	}

	userPosts, err := store.SearchPosts(context.Background(), "", "user")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(userPosts), 1; got != want {
		t.Fatalf("user posts = %d, want %d", got, want)
	}
	keywordPosts, err := store.SearchPosts(context.Background(), "", "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(keywordPosts), 1; got != want {
		t.Fatalf("keyword posts = %d, want %d", got, want)
	}
}

func TestRefreshPostsFetchesTrackedUserKeywordSources(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/2/users/by":
			t.Fatal("tracked_users mode should not resolve users")
		case "/2/users/1/tweets":
			t.Fatal("tracked_users mode should not fetch user timelines")
		case "/2/tweets/search/recent":
			queries = append(queries, req.URL.Query().Get("query"))
			json.NewEncoder(w).Encode(samplePostsResponse("202", "1", "alice"))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	store := testStore(t)
	client := NewXClient("token")
	client.BaseURL = server.URL
	cfg := AppConfig{
		Users:              []string{"alice"},
		Keywords:           []string{"Go"},
		KeywordSearchMode:  KeywordSearchTrackedUsers,
		Language:           "ja",
		ExcludeReposts:     true,
		ExcludeReplies:     true,
		MaxPagesPerRefresh: 1,
	}

	result := RefreshPosts(context.Background(), store, client, cfg)
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %+v", result.Errors)
	}
	if got, want := result.Sources, 1; got != want {
		t.Fatalf("sources = %d, want %d", got, want)
	}
	if got, want := result.Fetched, 1; got != want {
		t.Fatalf("fetched = %d, want %d", got, want)
	}
	if got, want := len(queries), 1; got != want {
		t.Fatalf("queries = %d, want %d", got, want)
	}
	if got, want := queries[0], "from:alice Go lang:ja -is:retweet -is:reply"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	keywordPosts, err := store.SearchPosts(context.Background(), "", "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(keywordPosts), 1; got != want {
		t.Fatalf("keyword posts = %d, want %d", got, want)
	}
}

func TestTrackedUserKeywordQueriesPackUsersAndKeywords(t *testing.T) {
	cfg := AppConfig{
		Language:       "ja",
		ExcludeReposts: true,
		ExcludeReplies: true,
	}
	queries := buildTrackedUserKeywordQueries([]string{"alice", "bob"}, []string{"Go", "SQLite"}, cfg)
	if got, want := len(queries), 1; got != want {
		t.Fatalf("queries = %d, want %d: %+v", got, want, queries)
	}
	want := "(from:alice OR from:bob) (Go OR SQLite) lang:ja -is:retweet -is:reply"
	if queries[0] != want {
		t.Fatalf("query = %q, want %q", queries[0], want)
	}
}
