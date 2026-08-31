package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/starai/api/internal/runtime"
	"github.com/starai/api/internal/service"
)

func TestCreativeAgentClockIsDeterministic(t *testing.T) {
	clock := creativeAgentClockAt(map[string]interface{}{"agent_default_timezone": "Asia/Shanghai"}, time.Date(2026, 8, 30, 14, 17, 18, 0, time.UTC))
	reply := creativeAgentClockReply(clock)
	if reply != "现在是 2026年08月30日 22:17:18，星期日（Asia/Shanghai，UTC+08:00）。" {
		t.Fatalf("unexpected clock reply: %s", reply)
	}
	if !isDirectClockQuestion("此时此刻是什么时间？") || !isDirectClockQuestion("这个时间偏差很大，北京时间是22:18") {
		t.Fatal("clock questions were not recognized")
	}
	if isDirectClockQuestion("生成一张当前时间主题海报") || isDirectClockQuestion("目前 TikTok 最热门的视频是什么？") {
		t.Fatal("non-clock request was recognized as clock question")
	}
	tokyo, ok := creativeAgentClockForQuestion("东京现在是什么时间？", clock)
	if !ok || !strings.Contains(creativeAgentClockReply(tokyo), "23:17:18") {
		t.Fatalf("unexpected Tokyo clock: %#v", tokyo)
	}
	honolulu, ok := creativeAgentClockForQuestion("檀香山现在几点？", clock)
	if !ok || !strings.Contains(creativeAgentClockReply(honolulu), "04:17:18") {
		t.Fatalf("unexpected Honolulu clock: %#v", honolulu)
	}
	if _, ok := creativeAgentClockForQuestion("火星基地现在几点？", clock); ok {
		t.Fatal("unknown location must not use the default timezone")
	}
}

func TestCreativeSearchDecisionParsingAndFallback(t *testing.T) {
	decision, ok := parseCreativeSearchDecision("```json\n{\"needs_search\":true,\"query\":\"TikTok trending videos August 2026\",\"topic\":\"news\",\"time_range\":\"week\",\"include_domains\":[\"tiktok.com\"]}\n```")
	if !ok || !decision.NeedsSearch || decision.Topic != "news" || decision.TimeRange != "week" || len(decision.IncludeDomains) != 1 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	clock := creativeAgentClockAt(nil, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	decision = normalizeCreativeSearchDecision(decision, "请只搜索 tiktok.com 的近期热门视频", clock)
	if len(decision.IncludeDomains) != 1 || decision.IncludeDomains[0] != "tiktok.com" {
		t.Fatalf("explicit domain was not retained: %#v", decision)
	}
	decision = normalizeCreativeSearchDecision(decision, "请搜索近期热门视频", clock)
	if len(decision.IncludeDomains) != 0 {
		t.Fatalf("hallucinated domain was retained: %#v", decision)
	}
	fallback := defaultCreativeSearchDecision("目前 TikTok 最火爆的短视频", clock)
	if fallback.TimeRange != "week" || !strings.Contains(fallback.Query, "2026-08-30") {
		t.Fatalf("unexpected fallback: %#v", fallback)
	}
	today := defaultCreativeSearchDecision("今天有什么人工智能新闻", clock)
	if today.TimeRange != "day" || !strings.Contains(today.Query, "2026-08-30") {
		t.Fatalf("today query must include the full date: %#v", today)
	}
	global := defaultCreativeSearchDecision("此时的全球热点新闻", clock)
	if global.TimeRange != "day" || global.Topic != "news" || global.Query != "August 30 2026 latest world breaking news global headlines" {
		t.Fatalf("global breaking news query was not optimized: %#v", global)
	}
	phoenix := defaultCreativeSearchDecision("给我整一份凤凰网上此时的热搜新闻资讯", clock)
	if len(phoenix.IncludeDomains) != 1 || phoenix.IncludeDomains[0] != "ifeng.com" {
		t.Fatalf("Phoenix News domain was not recognized: %#v", phoenix)
	}
	phoenixTech := defaultCreativeSearchDecision("帮我整理凤凰网科技类最新新闻", clock)
	if len(phoenixTech.IncludeDomains) != 1 || phoenixTech.IncludeDomains[0] != "tech.ifeng.com" {
		t.Fatalf("Phoenix technology section was not recognized: %#v", phoenixTech)
	}
	mainstreamSites := []struct {
		query string
		want  string
	}{
		{"整理新华社和央视新闻今天的要闻", "news.cn,xinhuanet.com,cctv.com"},
		{"汇总财新网、第一财经和华尔街见闻的财经新闻", "caixin.com,yicai.com,wallstreetcn.com"},
		{"查看36氪、机器之心和IT之家科技资讯", "36kr.com,jiqizhixin.com,ithome.com"},
		{"整理微博热搜和小红书热门话题", "weibo.com,xiaohongshu.com"},
		{"Summarize Reuters, BBC and TechCrunch news", "reuters.com,bbc.com,techcrunch.com"},
	}
	for _, item := range mainstreamSites {
		got := strings.Join(explicitDomainsInMessage(item.query), ",")
		if got != item.want {
			t.Fatalf("query %q: got %q, want %q", item.query, got, item.want)
		}
	}
	technology := defaultCreativeSearchDecision("帮我整理全球热搜的科技新闻", clock)
	if technology.Query != "August 30 2026 latest global technology news headlines" || technology.Topic != "news" {
		t.Fatalf("global technology query was not optimized: %#v", technology)
	}
	routed := normalizeCreativeSearchDecision(creativeSearchDecision{
		NeedsSearch: true, Query: "全球焦点新闻", Topic: "news",
	}, "此时的全球焦点新闻", clock)
	if routed.TimeRange != "day" || routed.Query != "August 30 2026 latest world breaking news global headlines" {
		t.Fatalf("router decision did not inherit current-day constraints: %#v", routed)
	}
}

func TestCreativeAgentFastSearchDecision(t *testing.T) {
	clock := creativeAgentClockAt(nil, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))
	decision, decided := creativeAgentFastSearchDecision([]runtime.ChatMessage{{Role: "user", Content: "帮我写一段产品介绍"}}, clock)
	if !decided || decision.NeedsSearch {
		t.Fatalf("creative request should bypass search routing: %#v", decision)
	}
	decision, decided = creativeAgentFastSearchDecision([]runtime.ChatMessage{{Role: "user", Content: "搜索 tiktok.com 今天最热门的视频"}}, clock)
	if !decided || !decision.NeedsSearch || decision.TimeRange != "day" || len(decision.IncludeDomains) != 1 || decision.IncludeDomains[0] != "tiktok.com" {
		t.Fatalf("explicit search should use fast path: %#v", decision)
	}
	_, decided = creativeAgentFastSearchDecision([]runtime.ChatMessage{{Role: "user", Content: "最近有哪些更新？"}, {Role: "assistant", Content: "这里是结果"}, {Role: "user", Content: "那国内呢？"}}, clock)
	if decided {
		t.Fatal("context-dependent follow-up should use the router model")
	}
	decision, decided = creativeAgentFastSearchDecision([]runtime.ChatMessage{
		{Role: "user", Content: "整理凤凰网当前热点"},
		{Role: "assistant", Content: "这里是热点"},
		{Role: "user", Content: "继续看看最新科技新闻"},
	}, clock)
	if !decided || len(decision.IncludeDomains) != 1 || decision.IncludeDomains[0] != "tech.ifeng.com" {
		t.Fatalf("fast follow-up did not retain the requested site: %#v", decision)
	}
	diagnostic := defaultCreativeSearchDecision("帮我验证一下智能搜索是否可用", clock)
	if diagnostic.Query != "2026-08-31 中国科技新闻" || diagnostic.Topic != "news" || diagnostic.TimeRange != "day" {
		t.Fatalf("search diagnostic must use a safe deterministic probe: %#v", diagnostic)
	}
}

func TestCreativeAgentSearchDomainContext(t *testing.T) {
	domains, context := explicitDomainsInMessages([]runtime.ChatMessage{
		{Role: "user", Content: "整理凤凰网当前热点"},
		{Role: "assistant", Content: "这里是热点"},
		{Role: "user", Content: "科技类呢？"},
	})
	retained := explicitSearchDomains(domains, context)
	if len(retained) != 1 || retained[0] != "tech.ifeng.com" {
		t.Fatalf("follow-up did not retain the source site: %#v", retained)
	}
}

func TestShouldSuggestCreativeAgentSearch(t *testing.T) {
	clock := creativeAgentClockAt(nil, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))
	cases := []struct {
		query string
		want  bool
	}{
		{"给我看看今天的时政新闻", true},
		{"现在黄金价格是多少", true},
		{"此时此刻是什么时间？", false},
		{"生成一张今天主题的海报", false},
		{"根据今天的新闻生成一张海报", true},
		{"帮我写一段产品介绍", false},
	}
	for _, item := range cases {
		got := shouldSuggestCreativeAgentSearch([]runtime.ChatMessage{{Role: "user", Content: item.query}}, clock)
		if got != item.want {
			t.Fatalf("query %q: got %v, want %v", item.query, got, item.want)
		}
	}
}

func TestCreativeAgentCitationValidation(t *testing.T) {
	plan := map[string]interface{}{"reply": "有效结论 [1]，无效结论 [9]。"}
	if warning := validateCreativeAgentCitations(plan, 2); warning != "" {
		t.Fatal(warning)
	}
	if plan["reply"] != "有效结论 [1]，无效结论 。" {
		t.Fatalf("unexpected reply: %v", plan["reply"])
	}
	plan = map[string]interface{}{"reply": "没有引用的回答"}
	if warning := validateCreativeAgentCitations(plan, 2); warning == "" {
		t.Fatal("missing citation warning")
	}
}

func TestCreativeAgentSearchPromptRequiresSynthesis(t *testing.T) {
	prompt := creativeAgentSearchPrompt(
		creativeSearchDecision{Query: "人工智能新闻", Topic: "news", TimeRange: "day"},
		[]service.WebSearchResult{{Title: "示例", URL: "https://example.com", Snippet: "摘要"}},
	)
	if !strings.Contains(prompt, "先交叉核验，再归纳、提炼并直接回答") || !strings.Contains(prompt, "必须给出具体条目及其要点") {
		t.Fatalf("search prompt must require a synthesized answer: %s", prompt)
	}
}

func TestCreativeAgentResearchRequestsUseMultipleSources(t *testing.T) {
	clock := creativeAgentClockAt(nil, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))
	requests := creativeAgentResearchRequests(
		creativeSearchDecision{Query: "August 31 2026 latest global technology news headlines", Topic: "news", TimeRange: "day"},
		clock,
	)
	if len(requests) != 3 || !strings.Contains(requests[1].Query, "人工智能") || !strings.Contains(requests[2].Query, "global technology") {
		t.Fatalf("unexpected technology research plan: %#v", requests)
	}
	google := creativeAgentResearchRequests(
		creativeSearchDecision{Query: "Google News 科技资讯 2026-08-31", Topic: "news", TimeRange: "day", IncludeDomains: []string{"news.google.com"}},
		clock,
	)
	if len(google) != 3 || len(google[0].IncludeDomains) != 1 || len(google[1].IncludeDomains) != 0 || len(google[2].IncludeDomains) != 0 {
		t.Fatalf("Google News must be discovered first and verified with publishers: %#v", google)
	}
}

func TestEnsureCreativeAgentSearchReplyReplacesEmptySummary(t *testing.T) {
	plan := map[string]interface{}{"reply": "已为你整理全球热搜科技新闻。"}
	ensureCreativeAgentSearchReply(plan, []service.WebSearchResult{{Title: "芯片进展", Snippet: "新一代芯片发布。"}})
	reply := stringAny(plan["reply"])
	if !strings.Contains(reply, "芯片进展") || !strings.Contains(reply, "[1]") {
		t.Fatalf("empty search summary was not repaired: %s", reply)
	}
}
