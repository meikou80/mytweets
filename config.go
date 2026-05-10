package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrConfigNotFound = errors.New("config.json not found")

const (
	KeywordSearchSeparate     = "separate"
	KeywordSearchTrackedUsers = "tracked_users"

	SearchEndpointRecent = "recent"
	SearchEndpointAll    = "all"
)

type AppConfig struct {
	Users              []string `json:"users"`
	Keywords           []string `json:"keywords"`
	KeywordSearchMode  string   `json:"keyword_search_mode"`
	SearchEndpoint     string   `json:"search_endpoint"`
	LookbackDays       int      `json:"lookback_days"`
	StartTime          string   `json:"start_time"`
	EndTime            string   `json:"end_time"`
	Language           string   `json:"language"`
	ExcludeReposts     bool     `json:"exclude_reposts"`
	ExcludeReplies     bool     `json:"exclude_replies"`
	MaxPagesPerRefresh int      `json:"max_pages_per_refresh"`
}

type rawConfig struct {
	Users              []string `json:"users"`
	Keywords           []string `json:"keywords"`
	KeywordSearchMode  string   `json:"keyword_search_mode"`
	SearchEndpoint     string   `json:"search_endpoint"`
	LookbackDays       int      `json:"lookback_days"`
	StartTime          string   `json:"start_time"`
	EndTime            string   `json:"end_time"`
	Language           string   `json:"language"`
	ExcludeReposts     *bool    `json:"exclude_reposts"`
	ExcludeReplies     *bool    `json:"exclude_replies"`
	MaxPagesPerRefresh int      `json:"max_pages_per_refresh"`
}

func LoadConfig(path string) (AppConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AppConfig{}, ErrConfigNotFound
		}
		return AppConfig{}, err
	}

	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return AppConfig{}, fmt.Errorf("parse config: %w", err)
	}
	return normalizeRawConfig(raw)
}

func defaultConfig() AppConfig {
	return AppConfig{
		Users:              []string{},
		Keywords:           []string{},
		KeywordSearchMode:  KeywordSearchSeparate,
		SearchEndpoint:     SearchEndpointRecent,
		ExcludeReposts:     true,
		ExcludeReplies:     true,
		MaxPagesPerRefresh: 1,
	}
}

func normalizeRawConfig(raw rawConfig) (AppConfig, error) {
	cfg := AppConfig{
		Users:              normalizeUsernames(raw.Users),
		Keywords:           normalizeStrings(raw.Keywords),
		KeywordSearchMode:  strings.TrimSpace(raw.KeywordSearchMode),
		SearchEndpoint:     strings.TrimSpace(raw.SearchEndpoint),
		LookbackDays:       raw.LookbackDays,
		StartTime:          strings.TrimSpace(raw.StartTime),
		EndTime:            strings.TrimSpace(raw.EndTime),
		Language:           strings.TrimSpace(raw.Language),
		ExcludeReposts:     true,
		ExcludeReplies:     true,
		MaxPagesPerRefresh: raw.MaxPagesPerRefresh,
	}
	if raw.ExcludeReposts != nil {
		cfg.ExcludeReposts = *raw.ExcludeReposts
	}
	if raw.ExcludeReplies != nil {
		cfg.ExcludeReplies = *raw.ExcludeReplies
	}
	return NormalizeConfig(cfg)
}

func NormalizeConfig(cfg AppConfig) (AppConfig, error) {
	cfg.Users = normalizeUsernames(cfg.Users)
	cfg.Keywords = normalizeStrings(cfg.Keywords)
	cfg.KeywordSearchMode = strings.TrimSpace(cfg.KeywordSearchMode)
	cfg.SearchEndpoint = strings.TrimSpace(cfg.SearchEndpoint)
	cfg.StartTime = strings.TrimSpace(cfg.StartTime)
	cfg.EndTime = strings.TrimSpace(cfg.EndTime)
	cfg.Language = strings.TrimSpace(cfg.Language)
	if cfg.KeywordSearchMode == "" {
		cfg.KeywordSearchMode = KeywordSearchSeparate
	}
	if cfg.SearchEndpoint == "" {
		cfg.SearchEndpoint = SearchEndpointRecent
	}
	if cfg.LookbackDays < 1 && cfg.SearchEndpoint == SearchEndpointAll && cfg.StartTime == "" {
		cfg.LookbackDays = 365
	}
	if cfg.MaxPagesPerRefresh < 1 {
		cfg.MaxPagesPerRefresh = 1
	}
	if len(cfg.Users) == 0 && len(cfg.Keywords) == 0 {
		return AppConfig{}, errors.New("config must contain at least one user or keyword")
	}
	if cfg.KeywordSearchMode != KeywordSearchSeparate && cfg.KeywordSearchMode != KeywordSearchTrackedUsers {
		return AppConfig{}, fmt.Errorf("invalid keyword_search_mode: %s", cfg.KeywordSearchMode)
	}
	if cfg.KeywordSearchMode == KeywordSearchTrackedUsers && (len(cfg.Users) == 0 || len(cfg.Keywords) == 0) {
		return AppConfig{}, errors.New("keyword_search_mode tracked_users requires at least one user and one keyword")
	}
	if cfg.SearchEndpoint != SearchEndpointRecent && cfg.SearchEndpoint != SearchEndpointAll {
		return AppConfig{}, fmt.Errorf("invalid search_endpoint: %s", cfg.SearchEndpoint)
	}
	return cfg, nil
}

func normalizeUsernames(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (cfg AppConfig) maxQueryLength() int {
	if cfg.SearchEndpoint == SearchEndpointAll {
		return 1024
	}
	return 512
}
