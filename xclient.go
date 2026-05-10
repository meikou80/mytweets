package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultXBaseURL = "https://api.x.com"

type XClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type XUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type XPost struct {
	ID        string `json:"id"`
	AuthorID  string `json:"author_id"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	Lang      string `json:"lang"`
	Username  string `json:"username"`
	RawJSON   string `json:"-"`
}

type XAPIError struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
	Type   string `json:"type"`
}

type xPostsMeta struct {
	NewestID  string `json:"newest_id"`
	NextToken string `json:"next_token"`
}

type FetchResult struct {
	Posts    []XPost
	NewestID string
}

type xUsersResponse struct {
	Data   []XUser     `json:"data"`
	Errors []XAPIError `json:"errors"`
}

type xPostsResponse struct {
	Data     []json.RawMessage `json:"data"`
	Includes struct {
		Users []XUser `json:"users"`
	} `json:"includes"`
	Meta   xPostsMeta  `json:"meta"`
	Errors []XAPIError `json:"errors"`
}

func NewXClient(token string) *XClient {
	return &XClient{
		BaseURL: defaultXBaseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *XClient) ResolveUsers(ctx context.Context, usernames []string) (map[string]XUser, []XAPIError, error) {
	resolved := map[string]XUser{}
	if len(usernames) == 0 {
		return resolved, nil, nil
	}

	for start := 0; start < len(usernames); start += 100 {
		end := start + 100
		if end > len(usernames) {
			end = len(usernames)
		}
		values := url.Values{}
		values.Set("usernames", strings.Join(usernames[start:end], ","))
		values.Set("user.fields", "id,name,username")

		var resp xUsersResponse
		if err := c.getJSON(ctx, "/2/users/by", values, &resp); err != nil {
			return resolved, nil, err
		}
		for _, user := range resp.Data {
			resolved[strings.ToLower(user.Username)] = user
		}
		if len(resp.Errors) > 0 {
			return resolved, resp.Errors, nil
		}
	}
	return resolved, nil, nil
}

func (c *XClient) FetchUserPosts(ctx context.Context, user XUser, cfg AppConfig, sinceID string) (FetchResult, error) {
	values := url.Values{}
	values.Set("max_results", "100")
	values.Set("tweet.fields", "author_id,created_at,lang,referenced_tweets")
	values.Set("expansions", "author_id")
	values.Set("user.fields", "id,name,username")
	if sinceID != "" {
		values.Set("since_id", sinceID)
	}
	excludes := []string{}
	if cfg.ExcludeReplies {
		excludes = append(excludes, "replies")
	}
	if cfg.ExcludeReposts {
		excludes = append(excludes, "retweets")
	}
	if len(excludes) > 0 {
		values.Set("exclude", strings.Join(excludes, ","))
	}

	result, err := c.fetchPosts(ctx, "/2/users/"+url.PathEscape(user.ID)+"/tweets", values, cfg.MaxPagesPerRefresh)
	if err != nil {
		return FetchResult{}, err
	}
	for i := range result.Posts {
		if result.Posts[i].Username == "" {
			result.Posts[i].Username = user.Username
		}
	}
	return result, nil
}

func (c *XClient) FetchKeywordPosts(ctx context.Context, keyword string, cfg AppConfig, sinceID string) (FetchResult, error) {
	return c.fetchKeywordPosts(ctx, buildKeywordQuery(keyword, cfg), cfg, sinceID)
}

func (c *XClient) FetchTrackedUserKeywordPosts(ctx context.Context, username, keyword string, cfg AppConfig, sinceID string) (FetchResult, error) {
	return c.fetchKeywordPosts(ctx, buildTrackedUserKeywordQuery(username, keyword, cfg), cfg, sinceID)
}

func (c *XClient) FetchTrackedUserKeywordQuery(ctx context.Context, query string, cfg AppConfig, sinceID string) (FetchResult, error) {
	return c.fetchKeywordPosts(ctx, query, cfg, sinceID)
}

func (c *XClient) fetchKeywordPosts(ctx context.Context, query string, cfg AppConfig, sinceID string) (FetchResult, error) {
	values := url.Values{}
	values.Set("query", query)
	values.Set("max_results", cfg.searchMaxResults())
	values.Set("tweet.fields", "author_id,created_at,lang,referenced_tweets")
	values.Set("expansions", "author_id")
	values.Set("user.fields", "id,name,username")
	if cfg.SearchEndpoint == SearchEndpointRecent && sinceID != "" {
		values.Set("since_id", sinceID)
	}
	if cfg.SearchEndpoint == SearchEndpointAll {
		startTime, endTime := cfg.searchTimeRange(time.Now().UTC())
		if startTime != "" {
			values.Set("start_time", startTime)
		}
		if endTime != "" {
			values.Set("end_time", endTime)
		}
		return c.fetchPosts(ctx, "/2/tweets/search/all", values, cfg.MaxPagesPerRefresh)
	}
	return c.fetchPosts(ctx, "/2/tweets/search/recent", values, cfg.MaxPagesPerRefresh)
}

func (c *XClient) fetchPosts(ctx context.Context, path string, values url.Values, maxPages int) (FetchResult, error) {
	if maxPages < 1 {
		maxPages = 1
	}
	var result FetchResult
	for page := 0; page < maxPages; page++ {
		var resp xPostsResponse
		if err := c.getJSON(ctx, path, values, &resp); err != nil {
			return result, err
		}
		usersByID := map[string]XUser{}
		for _, user := range resp.Includes.Users {
			usersByID[user.ID] = user
		}
		for _, raw := range resp.Data {
			var post XPost
			if err := json.Unmarshal(raw, &post); err != nil {
				return result, err
			}
			if user, ok := usersByID[post.AuthorID]; ok {
				post.Username = user.Username
			}
			post.RawJSON = string(raw)
			result.Posts = append(result.Posts, post)
		}
		if result.NewestID == "" {
			result.NewestID = resp.Meta.NewestID
		}
		if resp.Meta.NextToken == "" {
			break
		}
		values.Set("pagination_token", resp.Meta.NextToken)
	}
	return result, nil
}

func (c *XClient) getJSON(ctx context.Context, path string, values url.Values, out any) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	u.Path = path
	u.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "Bearer "+c.Token)
	req.Header.Set("accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("X API returned %d: %s", resp.StatusCode, string(body))
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func buildKeywordQuery(keyword string, cfg AppConfig) string {
	parts := []string{quoteSearchTerm(keyword)}
	if cfg.Language != "" {
		parts = append(parts, "lang:"+cfg.Language)
	}
	if cfg.ExcludeReposts {
		parts = append(parts, "-is:retweet")
	}
	if cfg.ExcludeReplies {
		parts = append(parts, "-is:reply")
	}
	return strings.Join(parts, " ")
}

func buildTrackedUserKeywordQuery(username, keyword string, cfg AppConfig) string {
	queries := buildTrackedUserKeywordQueries([]string{username}, []string{keyword}, cfg)
	if len(queries) == 0 {
		return ""
	}
	return queries[0]
}

func buildTrackedUserKeywordQueries(usernames, keywords []string, cfg AppConfig) []string {
	userTerms := splitQueryTerms(usernames, "from:", false)
	keywordTerms := splitQueryTerms(keywords, "", true)
	if len(userTerms) == 0 || len(keywordTerms) == 0 {
		return nil
	}
	suffix := querySuffix(cfg)
	queries := []string{}

	for userStart := 0; userStart < len(userTerms); {
		userEnd := userStart + 1
		for userEnd <= len(userTerms) {
			candidateUsers := userTerms[userStart:userEnd]
			if len(buildCombinedTrackedQuery(candidateUsers, keywordTerms[:1], suffix)) > cfg.maxQueryLength() && len(candidateUsers) > 1 {
				userEnd--
				break
			}
			if userEnd == len(userTerms) {
				break
			}
			userEnd++
		}
		if userEnd > len(userTerms) {
			userEnd = len(userTerms)
		}
		userGroup := userTerms[userStart:userEnd]

		for keywordStart := 0; keywordStart < len(keywordTerms); {
			keywordEnd := keywordStart + 1
			for keywordEnd <= len(keywordTerms) {
				candidateKeywords := keywordTerms[keywordStart:keywordEnd]
				query := buildCombinedTrackedQuery(userGroup, candidateKeywords, suffix)
				if len(query) > cfg.maxQueryLength() && len(candidateKeywords) > 1 {
					keywordEnd--
					break
				}
				if keywordEnd == len(keywordTerms) {
					break
				}
				keywordEnd++
			}
			if keywordEnd > len(keywordTerms) {
				keywordEnd = len(keywordTerms)
			}
			queries = append(queries, buildCombinedTrackedQuery(userGroup, keywordTerms[keywordStart:keywordEnd], suffix))
			keywordStart = keywordEnd
		}
		userStart = userEnd
	}
	return queries
}

func quoteSearchTerm(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return term
	}
	if strings.HasPrefix(term, "\"") || strings.HasPrefix(term, "#") || strings.HasPrefix(term, "@") {
		return term
	}
	if strings.ContainsAny(term, " \t\r\n") {
		return strconv.Quote(term)
	}
	return term
}

func splitQueryTerms(values []string, prefix string, quote bool) []string {
	terms := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
		if value == "" {
			continue
		}
		if quote {
			value = quoteSearchTerm(value)
		}
		terms = append(terms, prefix+value)
	}
	return terms
}

func buildCombinedTrackedQuery(users, keywords []string, suffix string) string {
	return strings.TrimSpace(groupedTerms(users) + " " + groupedTerms(keywords) + " " + suffix)
}

func groupedTerms(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 {
		return terms[0]
	}
	return "(" + strings.Join(terms, " OR ") + ")"
}

func querySuffix(cfg AppConfig) string {
	parts := []string{}
	if cfg.Language != "" {
		parts = append(parts, "lang:"+cfg.Language)
	}
	if cfg.ExcludeReposts {
		parts = append(parts, "-is:retweet")
	}
	if cfg.ExcludeReplies {
		parts = append(parts, "-is:reply")
	}
	return strings.Join(parts, " ")
}

func (cfg AppConfig) searchMaxResults() string {
	if cfg.SearchEndpoint == SearchEndpointAll {
		return "500"
	}
	return "100"
}

func (cfg AppConfig) searchTimeRange(now time.Time) (string, string) {
	if cfg.SearchEndpoint != SearchEndpointAll {
		return "", ""
	}
	endTime := cfg.EndTime
	if endTime == "" {
		endTime = now.Add(-30 * time.Second).Format(time.RFC3339)
	}
	startTime := cfg.StartTime
	if startTime == "" && cfg.LookbackDays > 0 {
		startTime = now.AddDate(0, 0, -cfg.LookbackDays).Format(time.RFC3339)
	}
	return startTime, endTime
}
