package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestXClientFetchesUsersAndPosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/2/users/by":
			if got, want := req.URL.Query().Get("usernames"), "alice"; got != want {
				t.Fatalf("usernames = %q, want %q", got, want)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "1", "name": "Alice", "username": "alice"}},
			})
		case "/2/users/1/tweets":
			if got, want := req.URL.Query().Get("exclude"), "replies,retweets"; got != want {
				t.Fatalf("exclude = %q, want %q", got, want)
			}
			json.NewEncoder(w).Encode(samplePostsResponse("101", "1", "alice"))
		case "/2/tweets/search/recent":
			got := req.URL.Query().Get("query")
			if got != "Go lang:ja -is:retweet -is:reply" && got != "from:alice Go lang:ja -is:retweet -is:reply" {
				t.Fatalf("query = %q", got)
			}
			json.NewEncoder(w).Encode(samplePostsResponse("202", "2", "bob"))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client := NewXClient("token")
	client.BaseURL = server.URL
	users, apiErrors, err := client.ResolveUsers(reqContext(t), []string{"alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(apiErrors) != 0 {
		t.Fatalf("api errors = %+v", apiErrors)
	}
	alice := users["alice"]
	if alice.ID != "1" {
		t.Fatalf("alice id = %q, want 1", alice.ID)
	}

	cfg := AppConfig{Language: "ja", ExcludeReposts: true, ExcludeReplies: true, MaxPagesPerRefresh: 1}
	userPosts, err := client.FetchUserPosts(reqContext(t), alice, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := userPosts.Posts[0].Username, "alice"; got != want {
		t.Fatalf("username = %q, want %q", got, want)
	}
	keywordPosts, err := client.FetchKeywordPosts(reqContext(t), "Go", cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keywordPosts.Posts[0].Username, "bob"; got != want {
		t.Fatalf("username = %q, want %q", got, want)
	}
	trackedKeywordPosts, err := client.FetchTrackedUserKeywordPosts(reqContext(t), "alice", "Go", cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := trackedKeywordPosts.Posts[0].Username, "bob"; got != want {
		t.Fatalf("username = %q, want %q", got, want)
	}
}

func TestXClientFetchesFullArchiveSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/2/tweets/search/all":
			if got, want := req.URL.Query().Get("max_results"), "500"; got != want {
				t.Fatalf("max_results = %q, want %q", got, want)
			}
			if got := req.URL.Query().Get("since_id"); got != "" {
				t.Fatalf("since_id = %q, want empty", got)
			}
			if got, want := req.URL.Query().Get("start_time"), "2025-05-10T00:00:00Z"; got != want {
				t.Fatalf("start_time = %q, want %q", got, want)
			}
			if got, want := req.URL.Query().Get("end_time"), "2026-05-10T00:00:00Z"; got != want {
				t.Fatalf("end_time = %q, want %q", got, want)
			}
			json.NewEncoder(w).Encode(samplePostsResponse("303", "3", "carol"))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client := NewXClient("token")
	client.BaseURL = server.URL
	cfg := AppConfig{
		SearchEndpoint:     SearchEndpointAll,
		StartTime:          "2025-05-10T00:00:00Z",
		EndTime:            "2026-05-10T00:00:00Z",
		ExcludeReposts:     true,
		ExcludeReplies:     true,
		MaxPagesPerRefresh: 1,
	}
	posts, err := client.FetchKeywordPosts(reqContext(t), "Go", cfg, "123")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := posts.Posts[0].Username, "carol"; got != want {
		t.Fatalf("username = %q, want %q", got, want)
	}
}

func TestFullArchiveDefaultEndTimeIsSafelyInThePast(t *testing.T) {
	cfg := AppConfig{SearchEndpoint: SearchEndpointAll, LookbackDays: 365}
	now := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)

	startTime, endTime := cfg.searchTimeRange(now)

	if got, want := startTime, "2025-05-10T00:00:00Z"; got != want {
		t.Fatalf("start_time = %q, want %q", got, want)
	}
	if got, want := endTime, "2026-05-09T23:59:30Z"; got != want {
		t.Fatalf("end_time = %q, want %q", got, want)
	}
}

func samplePostsResponse(id, authorID, username string) map[string]any {
	return map[string]any{
		"data": []map[string]string{{
			"id":         id,
			"author_id":  authorID,
			"text":       "Go",
			"created_at": "2026-05-09T00:00:00Z",
			"lang":       "en",
		}},
		"includes": map[string]any{
			"users": []map[string]string{{"id": authorID, "name": username, "username": username}},
		},
		"meta": map[string]string{"newest_id": id},
	}
}

func reqContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
