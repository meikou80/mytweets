package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"unicode"
)

var (
	addr       = flag.String("a", ":8989", "server address")
	dbPath     = flag.String("db", "tweets.db", "database path")
	configPath = flag.String("config", "config.json", "initial config import path")
)

func main() {
	flag.Parse()

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open database error: %v", err)
	}
	defer store.Close()

	http.HandleFunc("/refresh", refreshHandler(store, *configPath))
	http.HandleFunc("/config", configHandler(store, *configPath))
	http.HandleFunc("/search", searchHandler(store))
	http.HandleFunc("/export.csv", exportHandler(store))
	http.Handle("/", http.FileServer(http.Dir("public")))

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func refreshHandler(store *Store, cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		cfg, err := configFromStoreOrFile(req.Context(), store, cfgPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		token := os.Getenv("X_BEARER_TOKEN")
		if token == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X_BEARER_TOKEN is not set"})
			return
		}

		client := NewXClient(token)
		result := RefreshPosts(req.Context(), store, client, cfg)
		writeJSON(w, http.StatusOK, result)
	}
}

func configHandler(store *Store, cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			cfg, err := configForDisplay(req.Context(), store, cfgPath)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, cfg)
		case http.MethodPut:
			var raw rawConfig
			if err := json.NewDecoder(req.Body).Decode(&raw); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parse config: " + err.Error()})
				return
			}
			cfg, err := normalizeRawConfig(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			cfg, err = store.SaveConfig(req.Context(), cfg)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, cfg)
		default:
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func configFromStoreOrFile(ctx context.Context, store *Store, cfgPath string) (AppConfig, error) {
	cfg, err := store.GetConfig(ctx)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, ErrConfigNotSaved) {
		return AppConfig{}, err
	}

	cfg, err = LoadConfig(cfgPath)
	if err != nil {
		return AppConfig{}, err
	}
	return store.SaveConfig(ctx, cfg)
}

func configForDisplay(ctx context.Context, store *Store, cfgPath string) (AppConfig, error) {
	cfg, err := configFromStoreOrFile(ctx, store, cfgPath)
	if errors.Is(err, ErrConfigNotFound) {
		return defaultConfig(), nil
	}
	return cfg, err
}

func searchHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		posts, err := store.SearchPosts(req.Context(), req.FormValue("q"), req.FormValue("source"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, posts)
	}
}

func exportHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		posts, err := store.SearchPosts(req.Context(), req.FormValue("q"), req.FormValue("source"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("content-type", "text/csv; charset=utf-8")
		w.Header().Set("content-disposition", `attachment; filename="posts.csv"`)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			log.Printf("write csv bom error: %v", err)
			return
		}

		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"id", "username", "created_at", "text", "url", "lang"}); err != nil {
			log.Printf("write csv header error: %v", err)
			return
		}
		for _, post := range posts {
			if err := cw.Write([]string{post.ID, post.Username, post.CreatedAt, compactText(post.Text), post.URL, post.Lang}); err != nil {
				log.Printf("write csv row error: %v", err)
				return
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			log.Printf("flush csv error: %v", err)
		}
	}
}

func compactText(s string) string {
	var b strings.Builder
	lastWasSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastWasSpace = false
	}
	return strings.TrimSpace(b.String())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json error: %v", err)
	}
}
