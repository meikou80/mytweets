package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaultsAndNormalize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"users": ["@XDevelopers", "XDevelopers", ""],
		"keywords": [" Go ", "Go", ""]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(cfg.Users), 1; got != want {
		t.Fatalf("len(users) = %d, want %d", got, want)
	}
	if got, want := cfg.Users[0], "XDevelopers"; got != want {
		t.Fatalf("user = %q, want %q", got, want)
	}
	if got, want := len(cfg.Keywords), 1; got != want {
		t.Fatalf("len(keywords) = %d, want %d", got, want)
	}
	if !cfg.ExcludeReposts || !cfg.ExcludeReplies {
		t.Fatal("exclude defaults should be true")
	}
	if got, want := cfg.MaxPagesPerRefresh, 1; got != want {
		t.Fatalf("max pages = %d, want %d", got, want)
	}
	if got, want := cfg.KeywordSearchMode, KeywordSearchSeparate; got != want {
		t.Fatalf("keyword search mode = %q, want %q", got, want)
	}
	if got, want := cfg.SearchEndpoint, SearchEndpointRecent; got != want {
		t.Fatalf("search endpoint = %q, want %q", got, want)
	}
}

func TestLoadConfigNotFound(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != ErrConfigNotFound {
		t.Fatalf("err = %v, want ErrConfigNotFound", err)
	}
}

func TestLoadConfigTrackedUsersModeRequiresUsers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"keywords": ["Go"],
		"keyword_search_mode": "tracked_users"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigFullArchiveDefaultsLookbackDays(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"keywords": ["Go"],
		"search_endpoint": "all"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.SearchEndpoint, SearchEndpointAll; got != want {
		t.Fatalf("search endpoint = %q, want %q", got, want)
	}
	if got, want := cfg.LookbackDays, 365; got != want {
		t.Fatalf("lookback days = %d, want %d", got, want)
	}
}
