package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshHandlerMissingConfig(t *testing.T) {
	store := testStore(t)
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	rec := httptest.NewRecorder()

	refreshHandler(store, filepath.Join(t.TempDir(), "missing.json")).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestRefreshHandlerMissingToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"keywords":["Go"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("X_BEARER_TOKEN", "")

	store := testStore(t)
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	rec := httptest.NewRecorder()

	refreshHandler(store, cfgPath).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), "X_BEARER_TOKEN") {
		t.Fatalf("body = %q, want token error", rec.Body.String())
	}
}

func TestSearchHandlerEmptyDB(t *testing.T) {
	store := testStore(t)
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader("q=Go&source=all"))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	searchHandler(store).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), "[]"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestExportHandlerWritesCSV(t *testing.T) {
	store := testStore(t)
	req := httptest.NewRequest(http.MethodGet, "/export.csv?q=&source=all", nil)
	rec := httptest.NewRecorder()

	exportHandler(store).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got := rec.Header().Get("content-type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "id,username,created_at,text,url,lang") {
		t.Fatalf("body = %q, want csv header", rec.Body.String())
	}
}

func TestCompactText(t *testing.T) {
	got := compactText("  Go\n\nSQLite\t  自社開発\r\nテスト  ")
	want := "Go SQLite 自社開発 テスト"
	if got != want {
		t.Fatalf("compactText() = %q, want %q", got, want)
	}
}
