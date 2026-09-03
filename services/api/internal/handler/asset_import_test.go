package handler

import (
	"encoding/json"
	"net"
	"net/url"
	"testing"
)

func TestAssetImportRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"file:///tmp/a.mp4", "http://user:pass@example.com/a.mp4", "http://"} {
		if _, err := validateImportURL(raw); err == nil {
			t.Fatalf("expected unsafe URL to fail: %s", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1"} {
		if !blockedImportIP(net.ParseIP(raw)) {
			t.Fatalf("expected private IP to be blocked: %s", raw)
		}
	}
	if _, err := validateImportURL("https://cdn.example.com/video.mp4"); err != nil {
		t.Fatalf("expected public URL shape to pass: %v", err)
	}
	shared, err := validateImportURL("3.28 复制打开抖音 https://v.douyin.com/AbCdEf/ 分享")
	if err != nil || shared.String() != "https://v.douyin.com/AbCdEf/" {
		t.Fatalf("expected copied share text to extract URL, got %v %v", shared, err)
	}
}

func TestExtractTikTokVideoURL(t *testing.T) {
	page := []byte(`<html><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"itemInfo":{"itemStruct":{"video":{"playAddr":"https:\u002F\u002Fv16-webapp-prime.tiktok.com\u002Fvideo\u002Fsample.mp4","downloadAddr":"https:\u002F\u002Fexample.com\u002Fwatermarked.mp4"}}}}}}</script></html>`)
	got, err := extractTikTokVideoURL(page)
	if err != nil {
		t.Fatalf("extract TikTok video URL: %v", err)
	}
	if got != "https://v16-webapp-prime.tiktok.com/video/sample.mp4" {
		t.Fatalf("unexpected TikTok video URL: %q", got)
	}
}

func TestExtractTikTokVideoURLFromURLList(t *testing.T) {
	page := []byte(`<script type="application/json">{"video":{"playAddr":{"UrlList":["https://cdn.example.com/video.mp4"]}}}</script>`)
	got, err := extractTikTokVideoURL(page)
	if err != nil || got != "https://cdn.example.com/video.mp4" {
		t.Fatalf("unexpected result: url=%q err=%v", got, err)
	}
	if !isTikTokHost("www.tiktok.com") || !isTikTokHost("vt.tiktok.com") || isTikTokHost("tiktok.com.example.org") {
		t.Fatal("TikTok host validation failed")
	}
}

func TestDouyinLinkAndMetadataParsing(t *testing.T) {
	if !isDouyinHost("v.douyin.com") || !isDouyinHost("www.iesdouyin.com") || isDouyinHost("douyin.com.example.org") {
		t.Fatal("Douyin host validation failed")
	}
	for _, raw := range []string{
		"https://www.douyin.com/video/7372484719365098803",
		"https://www.iesdouyin.com/share/video/7372484719365098803/",
		"https://www.douyin.com/?modal_id=7372484719365098803",
	} {
		parsed, err := url.Parse(raw)
		if err != nil || douyinAwemeID(parsed) != "7372484719365098803" {
			t.Fatalf("failed to extract Douyin aweme id from %q", raw)
		}
	}
	var metadata interface{}
	if err := json.Unmarshal([]byte(`{"data":{"aweme_detail":{"video":{"play_addr":{"url_list":["https://cdn.example.com/video.mp4"]}}}}}`), &metadata); err != nil {
		t.Fatal(err)
	}
	if got := findTikTokURLByKey(metadata, "play_addr"); got != "https://cdn.example.com/video.mp4" {
		t.Fatalf("unexpected Douyin video URL: %q", got)
	}
}

func TestSocialVideoPlatforms(t *testing.T) {
	tests := map[string]string{
		"www.tiktok.com":      "TikTok",
		"fb.watch":            "Facebook",
		"www.facebook.com":    "Facebook",
		"instagram.com":       "Instagram",
		"m.youtube.com":       "YouTube",
		"youtu.be":            "YouTube",
		"x.com":               "X",
		"mobile.twitter.com":  "X",
		"v.douyin.com":        "抖音",
		"www.xiaohongshu.com": "小红书",
		"xhslink.com":         "小红书",
	}
	for host, want := range tests {
		if got := socialVideoPlatform(host); got != want {
			t.Errorf("socialVideoPlatform(%q) = %q, want %q", host, got, want)
		}
	}
	for _, host := range []string{"example.com", "youtube.com.example.org", "notx.com"} {
		if got := socialVideoPlatform(host); got != "" {
			t.Errorf("expected unsupported host %q, got %q", host, got)
		}
	}
}
