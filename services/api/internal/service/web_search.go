package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type WebSearchConfig struct {
	Enabled     bool
	Provider    string
	APIKey      string
	BaseURL     string
	SearchDepth string
	MaxResults  int
	TimeoutSec  int
	DailyLimit  int
	CacheTTLSec int
}

type WebSearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Snippet       string  `json:"snippet"`
	PublishedDate string  `json:"published_date,omitempty"`
	Score         float64 `json:"score,omitempty"`
}

type WebSearchRequest struct {
	Query          string
	Topic          string
	TimeRange      string
	IncludeDomains []string
}

func ParseWebSearchConfig(values map[string]interface{}) WebSearchConfig {
	cfg := WebSearchConfig{
		Enabled:     boolConfigValue(values["web_search_enabled"]),
		Provider:    strings.ToLower(strings.TrimSpace(stringConfigValue(values["web_search_provider"]))),
		APIKey:      strings.TrimSpace(stringConfigValue(values["web_search_api_key"])),
		BaseURL:     strings.TrimSpace(stringConfigValue(values["web_search_base_url"])),
		SearchDepth: strings.ToLower(strings.TrimSpace(stringConfigValue(values["web_search_depth"]))),
		MaxResults:  intConfigValue(values["web_search_max_results"], 5),
		TimeoutSec:  intConfigValue(values["web_search_timeout_sec"], 12),
		DailyLimit:  intConfigValue(values["web_search_daily_limit"], 100),
		CacheTTLSec: intConfigValue(values["web_search_cache_ttl_sec"], 600),
	}
	if cfg.Provider == "" {
		cfg.Provider = "tavily"
	}
	if cfg.SearchDepth != "advanced" {
		cfg.SearchDepth = "basic"
	}
	if cfg.MaxResults < 1 {
		cfg.MaxResults = 1
	} else if cfg.MaxResults > 10 {
		cfg.MaxResults = 10
	}
	if cfg.TimeoutSec < 3 {
		cfg.TimeoutSec = 3
	} else if cfg.TimeoutSec > 30 {
		cfg.TimeoutSec = 30
	}
	if cfg.DailyLimit < 0 {
		cfg.DailyLimit = 0
	}
	if cfg.CacheTTLSec < 0 {
		cfg.CacheTTLSec = 0
	} else if cfg.CacheTTLSec > 3600 {
		cfg.CacheTTLSec = 3600
	}
	return cfg
}

func ValidateWebSearchConfig(cfg WebSearchConfig) error {
	switch cfg.Provider {
	case "tavily", "brave":
		if cfg.Enabled && cfg.APIKey == "" {
			return errors.New("启用联网搜索前请填写搜索服务 API Key")
		}
	case "searxng":
		if cfg.Enabled && cfg.BaseURL == "" {
			return errors.New("启用 SearXNG 前请填写服务地址")
		}
		if cfg.BaseURL != "" {
			if _, err := searchEndpoint(cfg.BaseURL, "/search"); err != nil {
				return errors.New("SearXNG 服务地址必须是有效的 HTTP/HTTPS 地址")
			}
		}
	default:
		return errors.New("联网搜索服务商参数错误")
	}
	return nil
}

func SearchWeb(ctx context.Context, cfg WebSearchConfig, query string) ([]WebSearchResult, error) {
	return SearchWebWithOptions(ctx, cfg, WebSearchRequest{Query: query})
}

func SearchWebWithOptions(ctx context.Context, cfg WebSearchConfig, input WebSearchRequest) ([]WebSearchResult, error) {
	input = normalizeWebSearchRequest(input)
	if !cfg.Enabled {
		return nil, errors.New("联网搜索尚未启用")
	}
	if input.Query == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if err := ValidateWebSearchConfig(cfg); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second}
	var results []WebSearchResult
	var err error
	switch cfg.Provider {
	case "tavily":
		results, err = searchTavily(ctx, client, cfg, input)
	case "brave":
		results, err = searchBrave(ctx, client, cfg, input)
	case "searxng":
		results, err = searchSearXNG(ctx, client, cfg, input)
	}
	if err != nil {
		return nil, err
	}
	results = cleanWebSearchResults(results, cfg.MaxResults, input.IncludeDomains)
	if len(results) == 0 {
		return nil, errors.New("搜索服务未返回有效结果")
	}
	return results, nil
}

func searchTavily(ctx context.Context, client *http.Client, cfg WebSearchConfig, input WebSearchRequest) ([]WebSearchResult, error) {
	payloadBody := map[string]interface{}{
		"query": input.Query, "search_depth": cfg.SearchDepth, "max_results": cfg.MaxResults,
		"include_answer": false, "include_raw_content": false, "topic": input.Topic,
	}
	if input.TimeRange != "" {
		payloadBody["time_range"] = input.TimeRange
	}
	if len(input.IncludeDomains) > 0 {
		payloadBody["include_domains"] = input.IncludeDomains
	}
	body, _ := json.Marshal(payloadBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	var payload struct {
		Results []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"published_date"`
		} `json:"results"`
	}
	if err := doSearchRequest(client, req, &payload); err != nil {
		return nil, err
	}
	results := make([]WebSearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		if item.Score > 0 && item.Score < 0.25 {
			continue
		}
		results = append(results, WebSearchResult{Title: item.Title, URL: item.URL, Snippet: item.Content, PublishedDate: item.PublishedDate, Score: item.Score})
	}
	return results, nil
}

func searchBrave(ctx context.Context, client *http.Client, cfg WebSearchConfig, input WebSearchRequest) ([]WebSearchResult, error) {
	endpoint, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	params := endpoint.Query()
	params.Set("q", searchQueryWithDomains(input))
	params.Set("count", strconv.Itoa(cfg.MaxResults))
	params.Set("text_decorations", "false")
	if input.TimeRange != "" {
		params.Set("freshness", map[string]string{"day": "pd", "week": "pw", "month": "pm", "year": "py"}[input.TimeRange])
	}
	endpoint.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", cfg.APIKey)
	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Age         string `json:"age"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := doSearchRequest(client, req, &payload); err != nil {
		return nil, err
	}
	results := make([]WebSearchResult, 0, len(payload.Web.Results))
	for _, item := range payload.Web.Results {
		results = append(results, WebSearchResult{Title: item.Title, URL: item.URL, Snippet: item.Description, PublishedDate: item.Age})
	}
	return results, nil
}

func searchSearXNG(ctx context.Context, client *http.Client, cfg WebSearchConfig, input WebSearchRequest) ([]WebSearchResult, error) {
	endpoint, err := searchEndpoint(cfg.BaseURL, "/search")
	if err != nil {
		return nil, err
	}
	type searXNGPayload struct {
		Results []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"publishedDate"`
		} `json:"results"`
	}
	search := func(category, timeRange string) (searXNGPayload, error) {
		requestURL := *endpoint
		params := requestURL.Query()
		params.Set("q", searchQueryWithDomains(input))
		params.Set("format", "json")
		params.Set("language", "auto")
		if category != "" {
			params.Set("categories", category)
		}
		if timeRange != "" {
			params.Set("time_range", timeRange)
		}
		requestURL.RawQuery = params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return searXNGPayload{}, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Header.Set("X-Real-IP", "127.0.0.1")
		var payload searXNGPayload
		err = doSearchRequest(client, req, &payload)
		return payload, err
	}
	hasUsableResults := func(payload searXNGPayload) bool {
		if len(input.IncludeDomains) == 0 {
			return len(payload.Results) > 0
		}
		for _, item := range payload.Results {
			if searchResultMatchesDomains(item.URL, input.IncludeDomains) {
				return true
			}
		}
		return false
	}

	category := ""
	if input.Topic == "news" {
		category = "news"
	}
	payload, err := search(category, input.TimeRange)
	if err != nil {
		return nil, err
	}
	if !hasUsableResults(payload) && input.TimeRange != "" {
		payload, err = search(category, "")
	}
	if err == nil && !hasUsableResults(payload) && category != "" {
		payload, err = search("", "")
	}
	if err != nil {
		return nil, err
	}
	results := make([]WebSearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		results = append(results, WebSearchResult{Title: item.Title, URL: item.URL, Snippet: item.Content, PublishedDate: item.PublishedDate, Score: item.Score})
	}
	return results, nil
}

func searchEndpoint(baseURL, path string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil {
		return nil, errors.New("invalid search service URL")
	}
	if endpoint.Path == "" || endpoint.Path == "/" {
		endpoint.Path = path
	}
	return endpoint, nil
}

func doSearchRequest(client *http.Client, req *http.Request, target interface{}) error {
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("联网搜索请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("联网搜索返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(target); err != nil {
		return fmt.Errorf("联网搜索响应解析失败: %w", err)
	}
	return nil
}

func cleanWebSearchResults(items []WebSearchResult, limit int, domains []string) []WebSearchResult {
	out := make([]WebSearchResult, 0, limit)
	seen := make(map[string]bool)
	hostCounts := make(map[string]int)
	for _, item := range items {
		item.Title = strings.TrimSpace(item.Title)
		item.URL = strings.TrimSpace(item.URL)
		item.Snippet = strings.TrimSpace(item.Snippet)
		item.PublishedDate = strings.TrimSpace(item.PublishedDate)
		parsed, err := url.Parse(item.URL)
		if err != nil || parsed == nil {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || seen[item.URL] || hostCounts[host] >= 2 || !searchResultMatchesDomains(item.URL, domains) {
			continue
		}
		seen[item.URL] = true
		hostCounts[host]++
		if item.Title == "" {
			item.Title = parsed.Host
		}
		if runes := []rune(item.Snippet); len(runes) > 1200 {
			item.Snippet = string(runes[:1200]) + "…"
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func searchResultMatchesDomains(rawURL string, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, domain := range domains {
		domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func normalizeWebSearchRequest(input WebSearchRequest) WebSearchRequest {
	input.Query = strings.TrimSpace(input.Query)
	input.Topic = strings.ToLower(strings.TrimSpace(input.Topic))
	if input.Topic != "news" && input.Topic != "finance" {
		input.Topic = "general"
	}
	input.TimeRange = strings.ToLower(strings.TrimSpace(input.TimeRange))
	if input.TimeRange != "day" && input.TimeRange != "week" && input.TimeRange != "month" && input.TimeRange != "year" {
		input.TimeRange = ""
	}
	domains := make([]string, 0, len(input.IncludeDomains))
	seen := make(map[string]bool)
	for _, domain := range input.IncludeDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		domain = strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
		domain = strings.Trim(domain, "/")
		if domain == "" || strings.ContainsAny(domain, " ?#") || seen[domain] {
			continue
		}
		seen[domain] = true
		domains = append(domains, domain)
		if len(domains) == 8 {
			break
		}
	}
	input.IncludeDomains = domains
	return input
}

func searchQueryWithDomains(input WebSearchRequest) string {
	query := input.Query
	for _, domain := range input.IncludeDomains {
		query += " site:" + domain
	}
	return query
}

func boolConfigValue(value interface{}) bool {
	switch item := value.(type) {
	case bool:
		return item
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(item))
		return parsed
	default:
		return false
	}
}

func stringConfigValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func intConfigValue(value interface{}, fallback int) int {
	switch item := value.(type) {
	case float64:
		return int(item)
	case int:
		return item
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(item)); err == nil {
			return parsed
		}
	}
	return fallback
}
