package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/starai/api/internal/util"
)

const maxImportedVideoBytes int64 = 500 << 20
const maxImportedPageBytes int64 = 10 << 20

const importBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
const defaultDouyinResolverURL = "https://douyin.wtf/api/douyin/web/fetch_one_video"

type importAssetURLInput struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

func (h *Handler) ImportAssetURL(c *gin.Context) {
	if h.storage == nil {
		util.InternalError(c, "对象存储未启用")
		return
	}
	var input importAssetURLInput
	if err := c.ShouldBindJSON(&input); err != nil {
		util.BadRequest(c, "请输入视频 URL")
		return
	}
	remoteURL, err := validateImportURL(input.URL)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	client := safeImportHTTPClient()
	resp, err := openImportVideo(c.Request.Context(), client, remoteURL)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxImportedVideoBytes {
		util.BadRequest(c, "视频文件不能超过 500MB")
		return
	}
	tmp, err := os.CreateTemp("", "starai-video-import-*")
	if err != nil {
		util.InternalError(c, "创建临时文件失败")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()
	size, err := io.Copy(tmp, io.LimitReader(resp.Body, maxImportedVideoBytes+1))
	if err != nil {
		util.BadRequest(c, "视频 URL 下载失败")
		return
	}
	if size > maxImportedVideoBytes {
		util.BadRequest(c, "视频文件不能超过 500MB")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		util.InternalError(c, "读取视频文件失败")
		return
	}
	head := make([]byte, 512)
	headSize, _ := io.ReadFull(tmp, head)
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	detectedType := http.DetectContentType(head[:headSize])
	if !strings.HasPrefix(strings.ToLower(contentType), "video/") && !strings.HasPrefix(strings.ToLower(detectedType), "video/") {
		util.BadRequest(c, "URL 返回的不是可识别的视频文件")
		return
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "video/") {
		contentType = detectedType
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		util.InternalError(c, "读取视频文件失败")
		return
	}
	durationSeconds := probeImportedVideoDuration(c.Request.Context(), tmpName)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = path.Base(remoteURL.Path)
	}
	if name == "" || name == "." || name == "/" {
		name = "imported-video.mp4"
	}
	if len([]rune(name)) > 50 {
		name = string([]rune(name)[:50])
	}
	fileName := filepath.Base(strings.NewReplacer("/", "_", "\\", "_").Replace(name))
	if filepath.Ext(fileName) == "" {
		switch strings.ToLower(contentType) {
		case "video/quicktime":
			fileName += ".mov"
		case "video/webm":
			fileName += ".webm"
		default:
			fileName += ".mp4"
		}
	}
	publicID := util.NewPublicID("ast")
	objectName := fmt.Sprintf("assets/%d/%s/%s", c.GetInt64("user_id"), publicID, fileName)
	assetURL, err := h.storage.Upload(c.Request.Context(), objectName, contentType, tmp, size)
	if err != nil {
		util.InternalError(c, "保存视频失败")
		return
	}
	mime := contentType
	if err := h.assets.Create(c.Request.Context(), c.GetInt64("user_id"), publicID, h.cfg.MinioBucket, objectName, &name, nil, "video", "prop", &mime, size, []string{}); err != nil {
		_ = h.storage.Delete(c.Request.Context(), objectName)
		util.InternalError(c, "保存资产失败")
		return
	}
	util.Created(c, map[string]interface{}{
		"public_id":        publicID,
		"name":             name,
		"kind":             "video",
		"asset_type":       "prop",
		"mime_type":        mime,
		"size_bytes":       size,
		"duration_seconds": durationSeconds,
		"url":              assetURL,
		"tags":             []string{},
		"created_at":       time.Now().Format(time.RFC3339),
	})
}

func validateImportURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if start := strings.Index(raw, "http"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.IndexAny(raw, " \t\r\n"); end >= 0 {
		raw = raw[:end]
	}
	raw = strings.Trim(raw, "\"'<>，。；;）)]】")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("仅支持公开的 HTTP/HTTPS 视频直链")
	}
	return u, nil
}

func safeImportHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, fmt.Errorf("无法解析视频地址")
		}
		for _, address := range addresses {
			if blockedImportIP(address.IP) {
				return nil, fmt.Errorf("禁止访问本机或内网地址")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Minute,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("视频 URL 重定向次数过多")
			}
			_, err := validateImportURL(req.URL.String())
			return err
		},
	}
}

func openImportVideo(ctx context.Context, client *http.Client, sourceURL *url.URL) (*http.Response, error) {
	resp, err := getImportURL(ctx, client, sourceURL.String(), "")
	if err != nil {
		return nil, fmt.Errorf("视频 URL 下载失败")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("视频 URL 返回 HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if strings.HasPrefix(contentType, "video/") || contentType == "application/octet-stream" {
		return resp, nil
	}
	finalURL := resp.Request.URL
	if isDouyinHost(sourceURL.Hostname()) || isDouyinHost(finalURL.Hostname()) {
		resp.Body.Close()
		return openDouyinVideo(ctx, client, sourceURL, finalURL)
	}
	if !isTikTokHost(sourceURL.Hostname()) && !isTikTokHost(finalURL.Hostname()) {
		return resp, nil
	}
	page, readErr := io.ReadAll(io.LimitReader(resp.Body, maxImportedPageBytes+1))
	resp.Body.Close()
	if readErr != nil || int64(len(page)) > maxImportedPageBytes {
		return nil, fmt.Errorf("TikTok 页面解析失败")
	}
	videoURL, parseErr := extractTikTokVideoURL(page)
	if parseErr != nil {
		return nil, parseErr
	}
	parsedVideoURL, validateErr := validateImportURL(videoURL)
	if validateErr != nil {
		return nil, fmt.Errorf("TikTok 视频地址无效")
	}
	videoResp, fetchErr := getImportURL(ctx, client, parsedVideoURL.String(), finalURL.String())
	if fetchErr != nil {
		return nil, fmt.Errorf("TikTok 视频下载失败")
	}
	if videoResp.StatusCode < 200 || videoResp.StatusCode >= 300 {
		videoResp.Body.Close()
		return nil, fmt.Errorf("TikTok 视频返回 HTTP %d", videoResp.StatusCode)
	}
	return videoResp, nil
}

func getImportURL(ctx context.Context, client *http.Client, rawURL, referer string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", importBrowserUserAgent)
	req.Header.Set("Accept", "video/webm,video/mp4,video/*;q=0.9,text/html;q=0.8,*/*;q=0.5")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return client.Do(req)
}

func isTikTokHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com")
}

func isDouyinHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "douyin.com" || strings.HasSuffix(host, ".douyin.com") || host == "iesdouyin.com" || strings.HasSuffix(host, ".iesdouyin.com")
}

func openDouyinVideo(ctx context.Context, client *http.Client, sourceURL, finalURL *url.URL) (*http.Response, error) {
	awemeID := douyinAwemeID(finalURL)
	if awemeID == "" {
		awemeID = douyinAwemeID(sourceURL)
	}
	if awemeID == "" {
		return nil, fmt.Errorf("未能识别抖音作品 ID，请确认分享链接完整且公开")
	}
	resolver := strings.TrimSpace(os.Getenv("DOUYIN_RESOLVER_URL"))
	if resolver == "" {
		resolver = defaultDouyinResolverURL
	}
	resolverURL, err := validateImportURL(resolver)
	if err != nil {
		return nil, fmt.Errorf("抖音解析服务地址无效")
	}
	query := resolverURL.Query()
	query.Set("aweme_id", awemeID)
	resolverURL.RawQuery = query.Encode()
	metadataResp, err := getImportURL(ctx, client, resolverURL.String(), finalURL.String())
	if err != nil {
		return nil, fmt.Errorf("抖音视频解析服务不可用")
	}
	defer metadataResp.Body.Close()
	if metadataResp.StatusCode < 200 || metadataResp.StatusCode >= 300 {
		return nil, fmt.Errorf("抖音视频解析返回 HTTP %d", metadataResp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(metadataResp.Body, maxImportedPageBytes+1))
	if err != nil || int64(len(payload)) > maxImportedPageBytes {
		return nil, fmt.Errorf("抖音视频解析结果无效")
	}
	var value interface{}
	if json.Unmarshal(payload, &value) != nil {
		return nil, fmt.Errorf("抖音视频解析结果无效")
	}
	videoURL := ""
	for _, key := range []string{"play_addr", "playAddr", "play_addr_h264"} {
		if videoURL = findTikTokURLByKey(value, key); videoURL != "" {
			break
		}
	}
	parsedVideoURL, err := validateImportURL(videoURL)
	if err != nil {
		return nil, fmt.Errorf("未能解析抖音视频，请确认作品公开且可播放")
	}
	videoResp, err := getImportURL(ctx, client, parsedVideoURL.String(), finalURL.String())
	if err != nil {
		return nil, fmt.Errorf("抖音视频下载失败")
	}
	if videoResp.StatusCode < 200 || videoResp.StatusCode >= 300 {
		videoResp.Body.Close()
		return nil, fmt.Errorf("抖音视频返回 HTTP %d", videoResp.StatusCode)
	}
	return videoResp, nil
}

func douyinAwemeID(sourceURL *url.URL) string {
	if sourceURL == nil {
		return ""
	}
	for _, key := range []string{"modal_id", "aweme_id", "item_id"} {
		if value := sourceURL.Query().Get(key); allDigits(value) && len(value) >= 15 {
			return value
		}
	}
	parts := strings.FieldsFunc(sourceURL.Path, func(r rune) bool { return r == '/' || r == '-' || r == '_' })
	for index := len(parts) - 1; index >= 0; index-- {
		if allDigits(parts[index]) && len(parts[index]) >= 15 {
			return parts[index]
		}
	}
	return ""
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func extractTikTokVideoURL(page []byte) (string, error) {
	source := string(page)
	for offset := 0; ; {
		start := strings.Index(strings.ToLower(source[offset:]), "<script")
		if start < 0 {
			break
		}
		start += offset
		bodyStart := strings.Index(source[start:], ">")
		if bodyStart < 0 {
			break
		}
		bodyStart += start + 1
		bodyEnd := strings.Index(strings.ToLower(source[bodyStart:]), "</script>")
		if bodyEnd < 0 {
			break
		}
		bodyEnd += bodyStart
		body := strings.TrimSpace(html.UnescapeString(source[bodyStart:bodyEnd]))
		if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
			var value interface{}
			if json.Unmarshal([]byte(body), &value) == nil {
				for _, key := range []string{"playAddr", "downloadAddr", "playApi"} {
					if candidate := findTikTokURLByKey(value, key); candidate != "" {
						return candidate, nil
					}
				}
			}
		}
		offset = bodyEnd + len("</script>")
	}
	return "", fmt.Errorf("未能解析 TikTok 视频，请确认链接公开且可访问")
}

func findTikTokURLByKey(value interface{}, wanted string) string {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, nested := range item {
			if strings.EqualFold(key, wanted) {
				if candidate := firstHTTPURL(nested); candidate != "" {
					return candidate
				}
			}
		}
		for _, nested := range item {
			if candidate := findTikTokURLByKey(nested, wanted); candidate != "" {
				return candidate
			}
		}
	case []interface{}:
		for _, nested := range item {
			if candidate := findTikTokURLByKey(nested, wanted); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func firstHTTPURL(value interface{}) string {
	switch item := value.(type) {
	case string:
		for _, candidate := range append([]string{item}, strings.Fields(item)...) {
			if parsed, err := validateImportURL(candidate); err == nil {
				return parsed.String()
			}
		}
	case []interface{}:
		for _, nested := range item {
			if candidate := firstHTTPURL(nested); candidate != "" {
				return candidate
			}
		}
	case map[string]interface{}:
		for _, key := range []string{"UrlList", "urlList", "url_list", "url", "src"} {
			if nested, ok := item[key]; ok {
				if candidate := firstHTTPURL(nested); candidate != "" {
					return candidate
				}
			}
		}
	}
	return ""
}

func probeImportedVideoDuration(ctx context.Context, fileName string) float64 {
	ffprobePath := "ffprobe"
	if configured := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); configured != "" {
		if info, err := os.Stat(configured); err == nil && info.IsDir() {
			ffprobePath = filepath.Join(configured, ffprobeExecutableNameForImport())
		} else {
			ffprobePath = filepath.Join(filepath.Dir(configured), ffprobeExecutableNameForImport())
		}
	} else if discovered, err := exec.LookPath(ffprobePath); err == nil {
		ffprobePath = discovered
	} else {
		return 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, ffprobePath,
		"-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", fileName,
	).Output()
	if err != nil {
		return 0
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || duration <= 0 {
		return 0
	}
	return float64(int(duration*10+0.5)) / 10
}

func ffprobeExecutableNameForImport() string {
	if filepath.Separator == '\\' {
		return "ffprobe.exe"
	}
	return "ffprobe"
}

func blockedImportIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return true
	}
	return false
}
