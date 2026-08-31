package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
