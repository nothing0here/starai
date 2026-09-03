package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchWebWithOfficialRedFoxArticleAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("REDFOX_API_KEY") != "redfox-test-key" {
			t.Fatalf("unexpected RedFox request: method=%s auth=%q", r.Method, r.Header.Get("REDFOX_API_KEY"))
		}
		if r.URL.Path != "/gateway/story/api/dyData/searchArticle" {
			t.Fatalf("unexpected RedFox path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["keyword"] != "科技" || body["offset"] != float64(0) || body["sortType"] != "default" {
			t.Fatalf("unexpected RedFox body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":2000,"msg":"成功","data":{"list":[{"title":"科技作品","content":"科技内容","workUrl":"https://www.douyin.com/video/123","publishTime":"2026-09-03"}]}}`))
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "redfox", RedFoxAPIKey: "redfox-test-key", RedFoxBaseURL: server.URL + "/gateway", MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "帮我查一下抖音科技视频"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "科技作品" || results[0].URL != "https://www.douyin.com/video/123" || results[0].Provider != "redfox" {
		t.Fatalf("unexpected RedFox results: %#v", results)
	}
}

func TestSearchWebWithOfficialRedFoxUserAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/story/api/dyData/searchUser" {
			t.Fatalf("unexpected RedFox path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["keyword"] != "人民日报" {
			t.Fatalf("unexpected keyword: %#v", body["keyword"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":2000,"data":{"list":[{"nickname":"人民日报","signature":"权威媒体账号","followerCount":120000000,"totalFavorited":800000000,"awemeCount":10000,"crawlTime":"2026-09-03"}]}}`))
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "redfox", RedFoxAPIKey: "key", RedFoxBaseURL: server.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "搜索抖音账号 人民日报"})
	if err != nil || len(results) != 1 || results[0].Title != "人民日报" || !strings.Contains(results[0].URL, "douyin.com/search/") || !strings.Contains(results[0].Snippet, "粉丝数") {
		t.Fatalf("unexpected RedFox user results: %#v err=%v", results, err)
	}
}

func TestRedFoxParsesNestedAndPlatformResults(t *testing.T) {
	payload := map[string]interface{}{
		"result": map[string]interface{}{
			"search_result": map[string]interface{}{
				"sources": []interface{}{map[string]interface{}{
					"name": "官方来源", "link": "https://example.com/source", "summary": "可核验内容",
				}},
			},
			"list": []interface{}{map[string]interface{}{
				"title": "抖音作品", "workUrl": "https://www.douyin.com/video/123", "content": "作品内容", "publishTime": "2026-09-03",
			}},
		},
	}
	results := redFoxWebResults(payload)
	urls := map[string]bool{}
	for _, result := range results {
		urls[result.URL] = true
	}
	if len(results) != 2 || !urls["https://example.com/source"] || !urls["https://www.douyin.com/video/123"] {
		t.Fatalf("nested RedFox results were not parsed: %#v", results)
	}
}

func TestRedFoxFallsBackToConfiguredSearXNG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/story/api/dyData/searchArticle":
			_, _ = w.Write([]byte(`{"code":2000,"data":{"list":[]}}`))
		case "/search":
			_, _ = w.Write([]byte(`{"results":[{"title":"抖音科技视频","url":"https://example.com/douyin-tech","content":"抖音科技视频来源"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "redfox", BaseURL: server.URL, RedFoxAPIKey: "key", RedFoxBaseURL: server.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "抖音科技视频"})
	if err != nil || len(results) != 1 || results[0].URL != "https://example.com/douyin-tech" {
		t.Fatalf("RedFox fallback failed: results=%#v err=%v", results, err)
	}
}

func TestRedFoxFallsBackToTavilyWhenSearXNGIsEmpty(t *testing.T) {
	redfoxAndSearX := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/story/api/dyData/searchArticle":
			_, _ = w.Write([]byte(`{"code":2000,"data":{"list":[]}}`))
		case "/search":
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer redfoxAndSearX.Close()
	tavily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tavily-key" {
			t.Fatalf("unexpected Tavily authorization: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"抖音科技视频","url":"https://example.com/douyin-tech","content":"抖音科技视频来源","score":0.9}]}`))
	}))
	defer tavily.Close()
	previousURL := tavilySearchURL
	tavilySearchURL = tavily.URL
	defer func() { tavilySearchURL = previousURL }()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "redfox", APIKey: "tavily-key", BaseURL: redfoxAndSearX.URL, RedFoxAPIKey: "redfox-key", RedFoxBaseURL: redfoxAndSearX.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "抖音科技视频"})
	if err != nil || len(results) != 1 || results[0].Provider != "tavily" {
		t.Fatalf("RedFox Tavily fallback failed: results=%#v err=%v", results, err)
	}
}

func TestRedFoxCompletedWithoutSourcesIsNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":2000,"data":{"list":[]}}`))
	}))
	defer server.Close()

	_, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "redfox", RedFoxAPIKey: "key", RedFoxBaseURL: server.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "抖音不存在的视频"})
	if !errors.Is(err, ErrWebSearchNoResults) {
		t.Fatalf("expected ErrWebSearchNoResults, got %v", err)
	}
}

func TestRedFoxConfigAndAPIError(t *testing.T) {
	cfg := ParseWebSearchConfig(map[string]interface{}{
		"web_search_enabled": true, "web_search_provider": "REDFOX", "web_search_redfox_api_key": "key", "web_search_redfox_engine": "doubao", "web_search_timeout_sec": float64(999),
	})
	if cfg.Provider != "redfox" || cfg.TimeoutSec != 30 || ValidateWebSearchConfig(cfg) != nil {
		t.Fatalf("unexpected RedFox config: %#v", cfg)
	}
	missingKey := cfg
	missingKey.RedFoxAPIKey = ""
	missingKey.APIKey = "a-tavily-key-must-not-be-reused"
	if ValidateWebSearchConfig(missingKey) == nil {
		t.Fatal("RedFox must require its dedicated API key")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":3201,"msg":"积分不足"}`))
	}))
	defer server.Close()
	var result map[string]interface{}
	err := redFoxPost(context.Background(), server.Client(), server.URL, "key", "/submit", map[string]string{"inquiryText": "test"}, &result)
	if err == nil || !strings.Contains(err.Error(), "3201") || !strings.Contains(err.Error(), "积分不足") {
		t.Fatalf("unexpected RedFox API error: %v", err)
	}
}

func TestSearchWebWithSearXNG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "StarAI" || r.URL.Query().Get("format") != "json" || r.URL.Query().Get("time_range") != "week" || r.URL.Query().Get("categories") != "news" || r.URL.Query().Get("safesearch") != "1" || r.URL.Query().Get("language") != "en" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"StarAI Result","url":"https://example.com/a","content":"Fresh StarAI result"},{"title":"Duplicate","url":"https://example.com/a","content":"Ignored"},{"title":"Unsafe","url":"javascript:alert(1)","content":"Ignored"}]}`))
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: server.URL, MaxResults: 5, TimeoutSec: 3}, WebSearchRequest{Query: "StarAI", Topic: "news", TimeRange: "week"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "StarAI Result" || results[0].Snippet != "Fresh StarAI result" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSearchWebWithSearXNGRelaxesUnsupportedFilters(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch requests {
		case 1:
			if r.URL.Query().Get("categories") != "news" || r.URL.Query().Get("time_range") != "day" {
				t.Fatalf("unexpected strict request: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
		case 2:
			if r.URL.Query().Get("categories") != "news" || r.URL.Query().Has("time_range") {
				t.Fatalf("unexpected time fallback: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
		case 3:
			if r.URL.Query().Has("categories") || r.URL.Query().Has("time_range") {
				t.Fatalf("unexpected category fallback: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"results":[{"title":"AI news fallback","url":"https://example.com/news","content":"AI news result"}]}`))
		default:
			t.Fatalf("unexpected request count: %d", requests)
		}
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: server.URL, MaxResults: 5, TimeoutSec: 3}, WebSearchRequest{Query: "AI news", Topic: "news", TimeRange: "day"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(results) != 1 || results[0].Title != "AI news fallback" {
		t.Fatalf("unexpected fallback results: requests=%d results=%#v", requests, results)
	}
}

func TestSearchWebWithSearXNGEnforcesRequestedDomain(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("q") != "凤凰网 科技 新闻" {
			t.Fatalf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		switch requests {
		case 1:
			_, _ = w.Write([]byte(`{"results":[{"title":"百度地图","url":"https://map.baidu.com/a","content":"wrong domain"}]}`))
		case 2:
			_, _ = w.Write([]byte(`{"results":[{"title":"凤凰科技","url":"https://tech.ifeng.com/a","content":"right domain"},{"title":"高德地图","url":"https://amap.com/a","content":"wrong domain"}]}`))
		default:
			t.Fatalf("unexpected request count: %d", requests)
		}
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: server.URL, MaxResults: 5, TimeoutSec: 3}, WebSearchRequest{Query: "凤凰网 科技新闻", Topic: "news", TimeRange: "day", IncludeDomains: []string{"ifeng.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(results) != 1 || results[0].Title != "凤凰科技" {
		t.Fatalf("unexpected domain-filtered results: requests=%d results=%#v", requests, results)
	}
}

func TestParseWebSearchConfigBounds(t *testing.T) {
	cfg := ParseWebSearchConfig(map[string]interface{}{
		"web_search_enabled": true, "web_search_provider": "BRAVE", "web_search_max_results": float64(99), "web_search_timeout_sec": float64(1), "web_search_daily_limit": float64(-2), "web_search_cache_ttl_sec": float64(9999), "web_search_unit_price": "0.001",
	})
	if !cfg.Enabled || cfg.Provider != "brave" || cfg.MaxResults != 10 || cfg.TimeoutSec != 3 || cfg.DailyLimit != 0 || cfg.CacheTTLSec != 3600 || cfg.UnitPrice != 0.001 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	cfg.UnitPrice = -0.01
	if ValidateWebSearchConfig(cfg) == nil {
		t.Fatal("negative search price must be rejected")
	}
}

func TestWebSearchResultSafetyAndRelevance(t *testing.T) {
	input := WebSearchRequest{Query: "贵阳天气预报"}
	results := cleanWebSearchResults([]WebSearchResult{
		{Title: "Free Quiz Widget", URL: "https://example.com/quiz", Snippet: "Build a quiz widget"},
		{Title: "Test query for encyclopedia backstage", URL: "https://jobs.example.com/backstage", Snippet: "Engine diagnostic result"},
		{Title: "贵阳天气预报", URL: "https://weather.example.com/guiyang", Snippet: "贵阳今日气温"},
		{Title: "Pokemon hentai", URL: "https://newgrounds.com/adult", Snippet: "NSFW content"},
	}, 5, input)
	if len(results) != 1 || results[0].Title != "贵阳天气预报" {
		t.Fatalf("unsafe or irrelevant results were not filtered: %#v", results)
	}
}

func TestRedFoxResultUsesProviderSideRelevance(t *testing.T) {
	items := make([]WebSearchResult, 0, 5)
	for index := 0; index < 5; index++ {
		items = append(items, WebSearchResult{
			Title: "作品标题", URL: fmt.Sprintf("https://www.douyin.com/video/%d", index), Snippet: "RedFox 按关键词返回的作品", Provider: "redfox",
		})
	}
	results := cleanWebSearchResults(items, 5, WebSearchRequest{Query: "抖音热搜视频"})
	if len(results) != 5 {
		t.Fatalf("RedFox provider result was incorrectly filtered: %#v", results)
	}
}

func TestHybridSearchFallsBackWhenSearXNGReturnsEngineSelfTest(t *testing.T) {
	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Test query for encyclopedia backstage","url":"https://jobs.example.com/backstage","content":"Technology jobs"}]}`))
	}))
	defer searx.Close()
	tavily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"全球科技新闻","url":"https://news.example.com/technology","content":"今日全球科技资讯","score":0.9}]}`))
	}))
	defer tavily.Close()

	previousURL := tavilySearchURL
	tavilySearchURL = tavily.URL
	defer func() { tavilySearchURL = previousURL }()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "hybrid", APIKey: "test", BaseURL: searx.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "全球科技新闻", Topic: "news", TimeRange: "day"})
	if err != nil || len(results) != 1 || results[0].Title != "全球科技新闻" {
		t.Fatalf("hybrid self-test fallback failed: results=%#v err=%v", results, err)
	}
}

func TestSearchQueryWithMultipleDomainsUsesOR(t *testing.T) {
	query := searchQueryWithDomains(WebSearchRequest{Query: "科技新闻", IncludeDomains: []string{"news.cn", "cctv.com"}})
	if query != "科技新闻 (site:news.cn OR site:cctv.com)" {
		t.Fatalf("unexpected multi-domain query: %s", query)
	}
}

func TestSearXNGDoesNotSendSiteOperator(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"凤凰科技","url":"https://tech.ifeng.com/a","content":"今日科技新闻"}]}`))
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "searxng", BaseURL: server.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "帮我整理一下凤凰网前5条今日新闻 给我", Topic: "news", IncludeDomains: []string{"ifeng.com"}})
	if err != nil || len(results) != 1 {
		t.Fatalf("unexpected result: %#v err=%v", results, err)
	}
	if query != "凤凰网 今日 新闻" {
		t.Fatalf("SearXNG query should rely on result-domain filtering, got %q", query)
	}
}

func TestHybridSearchFallsBackToTavily(t *testing.T) {
	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Unrelated quiz","url":"https://widgets.example.com/quiz","content":"quiz builder"}]}`))
	}))
	defer searx.Close()
	tavily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"贵阳天气预报","url":"https://weather.example.com/guiyang","content":"贵阳今日气温","score":0.9}]}`))
	}))
	defer tavily.Close()

	previousURL := tavilySearchURL
	tavilySearchURL = tavily.URL
	defer func() { tavilySearchURL = previousURL }()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "hybrid", APIKey: "test", BaseURL: searx.URL, MaxResults: 5, TimeoutSec: 3,
	}, WebSearchRequest{Query: "贵阳天气预报"})
	if err != nil || len(results) != 1 || results[0].Title != "贵阳天气预报" {
		t.Fatalf("hybrid fallback failed: results=%#v err=%v", results, err)
	}
}
