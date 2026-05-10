package main

import (
	"context"
	"strings"
)

const (
	sourceUser               = "user"
	sourceKeyword            = "keyword"
	sourceTrackedUserKeyword = "tracked_user_keyword"
)

type RefreshResult struct {
	Inserted int            `json:"inserted"`
	Updated  int            `json:"updated"`
	Fetched  int            `json:"fetched"`
	Sources  int            `json:"sources"`
	Details  []RefreshDetail `json:"details"`
	Errors   []RefreshError `json:"errors"`
}

type RefreshDetail struct {
	SourceKind string `json:"source_kind"`
	SourceKey  string `json:"source_key"`
	Fetched    int    `json:"fetched"`
}

type RefreshError struct {
	SourceKind string `json:"source_kind,omitempty"`
	SourceKey  string `json:"source_key,omitempty"`
	Error      string `json:"error"`
}

func RefreshPosts(ctx context.Context, store *Store, client *XClient, cfg AppConfig) RefreshResult {
	result := RefreshResult{
		Details: []RefreshDetail{},
		Errors: []RefreshError{},
	}

	if cfg.KeywordSearchMode != KeywordSearchTrackedUsers && len(cfg.Users) > 0 {
		users, apiErrors, err := client.ResolveUsers(ctx, cfg.Users)
		if err != nil {
			result.Errors = append(result.Errors, RefreshError{SourceKind: sourceUser, Error: err.Error()})
		}
		for _, apiErr := range apiErrors {
			result.Errors = append(result.Errors, RefreshError{SourceKind: sourceUser, Error: formatXAPIError(apiErr)})
		}
		for _, username := range cfg.Users {
			result.Sources++
			user, ok := users[strings.ToLower(username)]
			if !ok {
				result.Errors = append(result.Errors, RefreshError{
					SourceKind: sourceUser,
					SourceKey:  username,
					Error:      "user could not be resolved",
				})
				continue
			}
			if err := store.CacheUser(ctx, user); err != nil {
				result.Errors = append(result.Errors, RefreshError{SourceKind: sourceUser, SourceKey: username, Error: err.Error()})
				continue
			}
			refreshSource(ctx, store, &result, sourceUser, username, func(sinceID string) (FetchResult, error) {
				return client.FetchUserPosts(ctx, user, cfg, sinceID)
			})
		}
	}

	switch cfg.KeywordSearchMode {
	case KeywordSearchTrackedUsers:
		queries := buildTrackedUserKeywordQueries(cfg.Users, cfg.Keywords, cfg)
		for _, query := range queries {
			sourceKey := query
			result.Sources++
			refreshSource(ctx, store, &result, sourceTrackedUserKeyword, sourceKey, func(sinceID string) (FetchResult, error) {
				return client.FetchTrackedUserKeywordQuery(ctx, query, cfg, sinceID)
			})
		}
	default:
		for _, keyword := range cfg.Keywords {
			result.Sources++
			refreshSource(ctx, store, &result, sourceKeyword, keyword, func(sinceID string) (FetchResult, error) {
				return client.FetchKeywordPosts(ctx, keyword, cfg, sinceID)
			})
		}
	}

	return result
}

func refreshSource(ctx context.Context, store *Store, result *RefreshResult, sourceKind, sourceKey string, fetch func(string) (FetchResult, error)) {
	sinceID, err := store.GetSinceID(ctx, sourceKind, sourceKey)
	if err != nil {
		result.Errors = append(result.Errors, RefreshError{SourceKind: sourceKind, SourceKey: sourceKey, Error: err.Error()})
		return
	}

	fetched, err := fetch(sinceID)
	if err != nil {
		result.Errors = append(result.Errors, RefreshError{SourceKind: sourceKind, SourceKey: sourceKey, Error: err.Error()})
		return
	}
	result.Fetched += len(fetched.Posts)
	result.Details = append(result.Details, RefreshDetail{
		SourceKind: sourceKind,
		SourceKey:  sourceKey,
		Fetched:    len(fetched.Posts),
	})

	for _, xpost := range fetched.Posts {
		post := StoredPost{
			Post: Post{
				ID:        xpost.ID,
				AuthorID:  xpost.AuthorID,
				Username:  xpost.Username,
				Text:      xpost.Text,
				CreatedAt: xpost.CreatedAt,
				Lang:      xpost.Lang,
			},
			RawJSON: xpost.RawJSON,
		}
		saveResult, err := store.SavePost(ctx, post, sourceKind, sourceKey)
		if err != nil {
			result.Errors = append(result.Errors, RefreshError{SourceKind: sourceKind, SourceKey: sourceKey, Error: err.Error()})
			continue
		}
		if saveResult.Inserted {
			result.Inserted++
		}
		if saveResult.Updated {
			result.Updated++
		}
	}
	if err := store.SaveSinceID(ctx, sourceKind, sourceKey, fetched.NewestID); err != nil {
		result.Errors = append(result.Errors, RefreshError{SourceKind: sourceKind, SourceKey: sourceKey, Error: err.Error()})
	}
}

func formatXAPIError(err XAPIError) string {
	if err.Detail != "" {
		return err.Detail
	}
	if err.Title != "" {
		return err.Title
	}
	if err.Type != "" {
		return err.Type
	}
	return "X API error"
}
