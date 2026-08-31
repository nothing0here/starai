package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchWebWithSearXNG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "StarAI" || r.URL.Query().Get("format") != "json" || r.URL.Query().Get("time_range") != "week" || r.URL.Query().Get("categories") != "news" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Result","url":"https://example.com/a","content":"Fresh result"},{"title":"Duplicate","url":"https://example.com/a","content":"Ignored"},{"title":"Unsafe","url":"javascript:alert(1)","content":"Ignored"}]}`))
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: server.URL, MaxResults: 5, TimeoutSec: 3}, WebSearchRequest{Query: "StarAI", Topic: "news", TimeRange: "week"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "Result" || results[0].Snippet != "Fresh result" {
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
			_, _ = w.Write([]byte(`{"results":[{"title":"Fallback","url":"https://example.com/news","content":"Result"}]}`))
		default:
			t.Fatalf("unexpected request count: %d", requests)
		}
	}))
	defer server.Close()

	results, err := SearchWebWithOptions(context.Background(), WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: server.URL, MaxResults: 5, TimeoutSec: 3}, WebSearchRequest{Query: "AI news", Topic: "news", TimeRange: "day"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(results) != 1 || results[0].Title != "Fallback" {
		t.Fatalf("unexpected fallback results: requests=%d results=%#v", requests, results)
	}
}

func TestParseWebSearchConfigBounds(t *testing.T) {
	cfg := ParseWebSearchConfig(map[string]interface{}{
		"web_search_enabled": true, "web_search_provider": "BRAVE", "web_search_max_results": float64(99), "web_search_timeout_sec": float64(1), "web_search_daily_limit": float64(-2), "web_search_cache_ttl_sec": float64(9999),
	})
	if !cfg.Enabled || cfg.Provider != "brave" || cfg.MaxResults != 10 || cfg.TimeoutSec != 3 || cfg.DailyLimit != 0 || cfg.CacheTTLSec != 3600 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
