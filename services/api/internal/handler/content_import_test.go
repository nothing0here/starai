package handler

import (
	"net/url"
	"strings"
	"testing"
)

func TestExtractImportedContentPrefersArticleBody(t *testing.T) {
	source, _ := url.Parse("https://mp.weixin.qq.com/s/example")
	page := `<html><head><meta property="og:title" content="示例标题"><meta name="author" content="示例作者"><meta name="description" content="短摘要"></head><body><nav>导航噪音</nav><div id="js_content"><p>第一段正文内容，用于分析文章主题与结构。</p><p>第二段正文内容，用于验证不会只返回页面摘要。</p></div><footer>页脚噪音</footer></body></html>`
	result, err := extractImportedContent([]byte(page), source, "text/html")
	if err != nil {
		t.Fatal(err)
	}
	if result.Platform != "微信公众号" || result.Title != "示例标题" || result.Author != "示例作者" {
		t.Fatalf("unexpected metadata: %#v", result)
	}
	if !strings.Contains(result.Content, "第一段正文内容") || strings.Contains(result.Content, "导航噪音") {
		t.Fatalf("unexpected content: %q", result.Content)
	}
}

func TestImportedContentPlatform(t *testing.T) {
	cases := map[string]string{
		"mp.weixin.qq.com":    "微信公众号",
		"www.xiaohongshu.com": "小红书",
		"www.toutiao.com":     "今日头条",
	}
	for host, want := range cases {
		if got := importedContentPlatform(host); got != want {
			t.Fatalf("platform for %s: got %q want %q", host, got, want)
		}
	}
}
