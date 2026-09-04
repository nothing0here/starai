package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/starai/api/internal/util"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const maxImportedContentRunes = 20000

type importContentURLInput struct {
	URL string `json:"url"`
}

type importedContent struct {
	URL       string `json:"url"`
	Platform  string `json:"platform"`
	Title     string `json:"title"`
	Author    string `json:"author,omitempty"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func (h *Handler) ImportContentURL(c *gin.Context) {
	var input importContentURLInput
	if err := c.ShouldBindJSON(&input); err != nil {
		util.BadRequest(c, "请输入内容 URL")
		return
	}
	remoteURL, err := validateImportURL(input.URL)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if contentID := toutiaoContentID(remoteURL); contentID != "" {
		result, importErr := importToutiaoContent(ctx, remoteURL, contentID)
		if importErr != nil {
			util.BadRequest(c, importErr.Error())
			return
		}
		util.OK(c, result)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		util.BadRequest(c, "内容 URL 无效")
		return
	}
	req.Header.Set("User-Agent", importBrowserUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.7")
	resp, err := safeImportHTTPClient().Do(req)
	if err != nil {
		util.BadRequest(c, fmt.Sprintf("内容 URL 读取失败：%v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		util.BadRequest(c, fmt.Sprintf("内容页面返回 HTTP %d", resp.StatusCode))
		return
	}
	if resp.ContentLength > maxImportedPageBytes {
		util.BadRequest(c, "内容页面不能超过 10MB")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxImportedPageBytes+1))
	if err != nil {
		util.BadRequest(c, "内容页面读取失败")
		return
	}
	if int64(len(raw)) > maxImportedPageBytes {
		util.BadRequest(c, "内容页面不能超过 10MB")
		return
	}
	contentType := resp.Header.Get("Content-Type")
	lowerType := strings.ToLower(strings.Split(contentType, ";")[0])
	if lowerType != "" && lowerType != "text/html" && lowerType != "application/xhtml+xml" && lowerType != "text/plain" {
		util.BadRequest(c, "URL 返回的不是可识别的网页内容")
		return
	}
	decoded := raw
	if reader, decodeErr := charset.NewReader(bytes.NewReader(raw), contentType); decodeErr == nil {
		if value, readErr := io.ReadAll(io.LimitReader(reader, maxImportedPageBytes+1)); readErr == nil {
			decoded = value
		}
	}
	finalURL := remoteURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL
	}
	result, err := extractImportedContent(decoded, finalURL, lowerType)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, result)
}

func toutiaoContentID(sourceURL *url.URL) string {
	if sourceURL == nil || !hostMatches(sourceURL.Hostname(), "toutiao.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(sourceURL.Path, "/"), "/")
	if len(parts) < 2 || (parts[0] != "w" && parts[0] != "article") {
		return ""
	}
	id := parts[1]
	if len(id) < 10 || len(id) > 24 {
		return ""
	}
	for _, char := range id {
		if char < '0' || char > '9' {
			return ""
		}
	}
	return id
}

func importToutiaoContent(ctx context.Context, sourceURL *url.URL, contentID string) (importedContent, error) {
	detailURL, _ := url.Parse("https://www.toutiao.com/api/pc/detail/")
	query := detailURL.Query()
	query.Set("aid", "24")
	query.Set("app_name", "toutiao_web")
	query.Set("item_id", contentID)
	query.Set("group_id", contentID)
	detailURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL.String(), nil)
	if err != nil {
		return importedContent{}, fmt.Errorf("今日头条链接无效")
	}
	req.Header.Set("User-Agent", importBrowserUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	resp, err := safeImportHTTPClient().Do(req)
	if err != nil {
		return importedContent{}, fmt.Errorf("今日头条正文读取失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return importedContent{}, fmt.Errorf("今日头条正文接口返回 HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxImportedPageBytes+1))
	if err != nil || int64(len(raw)) > maxImportedPageBytes {
		return importedContent{}, fmt.Errorf("今日头条正文读取失败")
	}
	return extractToutiaoDetail(raw, sourceURL)
}

func extractToutiaoDetail(raw []byte, sourceURL *url.URL) (importedContent, error) {
	var payload struct {
		Message string `json:"message"`
		Data    struct {
			Content     string `json:"content"`
			PGCInfoCard struct {
				MediaInfo struct {
					Name string `json:"name"`
				} `json:"media_info"`
			} `json:"pgc_info_card"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || strings.TrimSpace(payload.Data.Content) == "" {
		return importedContent{}, fmt.Errorf("今日头条未返回可识别的正文")
	}
	content := ""
	var richContent struct {
		RichContent struct {
			Text string `json:"text"`
		} `json:"rich_content"`
	}
	if json.Unmarshal([]byte(payload.Data.Content), &richContent) == nil {
		content = normalizeImportedText(richContent.RichContent.Text)
	}
	if content == "" {
		parsed, err := extractImportedContent([]byte(payload.Data.Content), sourceURL, "text/html")
		if err != nil {
			return importedContent{}, fmt.Errorf("今日头条未返回可识别的正文")
		}
		content = parsed.Content
	}
	content, truncated := truncateImportedContent(content)
	if utf8.RuneCountInString(content) < 20 {
		return importedContent{}, fmt.Errorf("今日头条正文过短，无法用于内容拆解")
	}
	title := strings.TrimSpace(strings.SplitN(content, "\n", 2)[0])
	return importedContent{
		URL:       sourceURL.String(),
		Platform:  "今日头条",
		Title:     truncateImportedRunes(title, 100),
		Author:    truncateImportedRunes(normalizeImportedText(payload.Data.PGCInfoCard.MediaInfo.Name), 100),
		Content:   content,
		Truncated: truncated,
	}, nil
}

func extractImportedContent(page []byte, sourceURL *url.URL, contentType string) (importedContent, error) {
	platform := importedContentPlatform(sourceURL.Hostname())
	if contentType == "text/plain" {
		content := normalizeImportedText(string(page))
		content, truncated := truncateImportedContent(content)
		if utf8.RuneCountInString(content) < 20 {
			return importedContent{}, fmt.Errorf("页面正文过短，无法用于内容拆解")
		}
		return importedContent{URL: sourceURL.String(), Platform: platform, Title: sourceURL.Hostname(), Content: content, Truncated: truncated}, nil
	}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return importedContent{}, fmt.Errorf("网页内容解析失败")
	}
	var title, author, description string
	type candidate struct {
		priority int
		text     string
	}
	candidates := make([]candidate, 0, 4)
	var body *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			attrs := importedNodeAttrs(node)
			if tag == "body" {
				body = node
			}
			if tag == "meta" {
				key := strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"], attrs["itemprop"]))
				value := strings.TrimSpace(attrs["content"])
				switch key {
				case "og:title", "twitter:title", "title":
					if title == "" {
						title = value
					}
				case "author", "article:author", "og:article:author":
					if author == "" {
						author = value
					}
				case "description", "og:description", "twitter:description":
					if description == "" || utf8.RuneCountInString(value) > utf8.RuneCountInString(description) {
						description = value
					}
				}
			}
			if title == "" && (tag == "title" || tag == "h1") {
				title = importedNodeText(node)
			}
			if author == "" && (attrs["id"] == "js_name" || importedClassContains(attrs["class"], "author")) {
				author = importedNodeText(node)
			}
			if priority := importedContentPriority(tag, attrs); priority > 0 {
				if text := importedNodeText(node); text != "" {
					candidates = append(candidates, candidate{priority: priority, text: text})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	best := candidate{}
	for _, item := range candidates {
		if item.priority > best.priority || (item.priority == best.priority && utf8.RuneCountInString(item.text) > utf8.RuneCountInString(best.text)) {
			best = item
		}
	}
	content := best.text
	if utf8.RuneCountInString(content) < 80 && utf8.RuneCountInString(description) > utf8.RuneCountInString(content) {
		content = normalizeImportedText(description)
	}
	if content == "" && body != nil {
		content = importedNodeText(body)
	}
	content, truncated := truncateImportedContent(content)
	if utf8.RuneCountInString(content) < 20 {
		return importedContent{}, fmt.Errorf("未能提取页面正文，请确认链接公开且可直接访问")
	}
	if title == "" {
		title = sourceURL.Hostname()
	}
	return importedContent{
		URL:       sourceURL.String(),
		Platform:  platform,
		Title:     truncateImportedRunes(normalizeImportedText(title), 200),
		Author:    truncateImportedRunes(normalizeImportedText(author), 100),
		Content:   content,
		Truncated: truncated,
	}, nil
}

func importedContentPlatform(host string) string {
	switch {
	case hostMatches(host, "mp.weixin.qq.com"), hostMatches(host, "weixin.qq.com"):
		return "微信公众号"
	case hostMatches(host, "xiaohongshu.com"), hostMatches(host, "xhslink.com"):
		return "小红书"
	case hostMatches(host, "toutiao.com"):
		return "今日头条"
	default:
		return strings.ToLower(strings.TrimSpace(host))
	}
}

func importedContentPriority(tag string, attrs map[string]string) int {
	if attrs["id"] == "js_content" {
		return 100
	}
	if tag == "article" {
		return 90
	}
	if tag == "main" {
		return 80
	}
	for _, className := range []string{"rich_media_content", "article-content", "article_content", "article-body", "post-content", "post_content", "note-content", "note_content"} {
		if importedClassContains(attrs["class"], className) {
			return 70
		}
	}
	return 0
}

func importedNodeAttrs(node *html.Node) map[string]string {
	attrs := make(map[string]string, len(node.Attr))
	for _, attr := range node.Attr {
		attrs[strings.ToLower(attr.Key)] = strings.TrimSpace(attr.Val)
	}
	return attrs
}

func importedClassContains(classes, wanted string) bool {
	wanted = strings.ToLower(wanted)
	for _, className := range strings.Fields(strings.ToLower(classes)) {
		if className == wanted {
			return true
		}
	}
	return false
}

func importedNodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode {
			switch strings.ToLower(current.Data) {
			case "script", "style", "noscript", "svg", "template", "nav", "footer", "header", "aside", "form", "button":
				return
			case "br":
				builder.WriteByte('\n')
				return
			}
		}
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if current.Type == html.ElementNode {
			switch strings.ToLower(current.Data) {
			case "p", "div", "li", "section", "article", "main", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
				builder.WriteByte('\n')
			}
		}
	}
	walk(node)
	return normalizeImportedText(builder.String())
}

func normalizeImportedText(value string) string {
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || (len(clean) > 0 && clean[len(clean)-1] == line) {
			continue
		}
		clean = append(clean, line)
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}

func truncateImportedContent(value string) (string, bool) {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxImportedContentRunes {
		return string(runes), false
	}
	return string(runes[:maxImportedContentRunes]), true
}

func truncateImportedRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
