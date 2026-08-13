package runtime

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	chatProtocolOpenAI = "openai"
	chatProtocolClaude = "claude"
	chatProtocolGemini = "gemini"
)

type protocolMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

func chatProtocol(extra map[string]interface{}) string {
	conn, _ := extra["connection"].(map[string]interface{})
	protocol, _ := conn["protocol"].(string)
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "claude", "anthropic", "anthropic_messages", "claude_messages":
		return chatProtocolClaude
	case "gemini", "gemini_native", "google", "google_gemini":
		return chatProtocolGemini
	default:
		return chatProtocolOpenAI
	}
}

func connectionValue(extra map[string]interface{}, key, fallback string) string {
	conn, _ := extra["connection"].(map[string]interface{})
	if value, ok := conn[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func prepareChatRequest(endpoint string, req ChatRequest, extra map[string]interface{}) (string, []byte, string, error) {
	protocol := chatProtocol(extra)
	endpoint = chatEndpoint(protocol, endpoint, req.Model, req.Stream)
	var (
		body []byte
		err  error
	)
	switch protocol {
	case chatProtocolClaude:
		body, err = marshalClaudeRequest(req)
	case chatProtocolGemini:
		body, err = marshalGeminiRequest(req)
	default:
		body, err = marshalChatRequest(req)
	}
	return endpoint, body, protocol, err
}

func chatEndpoint(protocol, endpoint, model string, stream bool) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || (protocol == chatProtocolClaude && endpoint == "/v1/chat/completions") ||
		(protocol == chatProtocolGemini && endpoint == "/v1/chat/completions") {
		switch protocol {
		case chatProtocolClaude:
			endpoint = "/v1/messages"
		case chatProtocolGemini:
			endpoint = "/v1beta/models/{model}:generateContent"
		default:
			endpoint = "/v1/chat/completions"
		}
	}
	endpoint = strings.ReplaceAll(endpoint, "{model}", url.PathEscape(strings.TrimPrefix(model, "models/")))
	if protocol == chatProtocolGemini && stream && !strings.Contains(endpoint, "alt=") {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		endpoint += separator + "alt=sse"
	}
	return endpoint
}

func modelListEndpoint(protocol, configured string) string {
	if strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured)
	}
	if protocol == chatProtocolGemini {
		return "/v1beta/models"
	}
	return "/v1/models"
}

func applyChatProtocolHeaders(req *http.Request, protocol string, extra map[string]interface{}) {
	switch protocol {
	case chatProtocolClaude:
		req.Header.Set("anthropic-version", connectionValue(extra, "anthropic_version", "2023-06-01"))
	}
}

func protocolMessages(messages interface{}) ([]protocolMessage, error) {
	if messages == nil {
		return nil, nil
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	var result []protocolMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func marshalClaudeRequest(req ChatRequest) ([]byte, error) {
	messages, err := protocolMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"model":    req.Model,
		"messages": make([]interface{}, 0, len(messages)),
		"stream":   req.Stream,
	}
	var system []interface{}
	for _, message := range messages {
		role := message.Role
		if role == "system" || role == "developer" {
			system = append(system, claudeContentParts(message.Content)...)
			continue
		}
		if role != "user" && role != "assistant" {
			role = "user"
		}
		body["messages"] = append(body["messages"].([]interface{}), map[string]interface{}{
			"role":    role,
			"content": claudeContent(message.Content),
		})
	}
	if len(system) > 0 {
		if len(system) == 1 {
			if text, ok := system[0].(string); ok {
				body["system"] = text
			} else {
				body["system"] = system
			}
		} else {
			body["system"] = system
		}
	}
	maxTokens := firstInt(req.Extra, "max_tokens", "max_completion_tokens")
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	body["max_tokens"] = maxTokens
	copyClaudeOption(body, req.Extra, "temperature")
	copyClaudeOption(body, req.Extra, "top_p")
	copyClaudeOption(body, req.Extra, "top_k")
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if stop, ok := req.Extra["stop"]; ok {
		if text, ok := stop.(string); ok {
			body["stop_sequences"] = []string{text}
		} else {
			body["stop_sequences"] = stop
		}
	}
	for _, key := range []string{"tools", "tool_choice", "thinking", "metadata"} {
		copyClaudeOption(body, req.Extra, key)
	}
	return json.Marshal(body)
}

func marshalGeminiRequest(req ChatRequest) ([]byte, error) {
	messages, err := protocolMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{"contents": []interface{}{}}
	var systemParts []interface{}
	for _, message := range messages {
		parts := geminiParts(message.Content)
		if message.Role == "system" || message.Role == "developer" {
			systemParts = append(systemParts, parts...)
			continue
		}
		role := "user"
		if message.Role == "assistant" || message.Role == "model" {
			role = "model"
		}
		body["contents"] = append(body["contents"].([]interface{}), map[string]interface{}{"role": role, "parts": parts})
	}
	if len(systemParts) > 0 {
		body["systemInstruction"] = map[string]interface{}{"parts": systemParts}
	}
	generation := map[string]interface{}{}
	if req.Temperature != nil {
		generation["temperature"] = *req.Temperature
	}
	copyGeminiOption(generation, req.Extra, "top_p", "topP")
	copyGeminiOption(generation, req.Extra, "top_k", "topK")
	if maxTokens := firstInt(req.Extra, "max_completion_tokens", "max_tokens"); maxTokens > 0 {
		generation["maxOutputTokens"] = maxTokens
	}
	if stop, ok := req.Extra["stop"]; ok {
		if text, ok := stop.(string); ok {
			generation["stopSequences"] = []string{text}
		} else {
			generation["stopSequences"] = stop
		}
	}
	if len(generation) > 0 {
		body["generationConfig"] = generation
	}
	if tools, ok := geminiTools(req.Extra["tools"]); ok {
		body["tools"] = tools
	}
	if safety, ok := req.Extra["safetySettings"]; ok {
		body["safetySettings"] = safety
	}
	return json.Marshal(body)
}

func claudeContent(content interface{}) interface{} {
	if text, ok := content.(string); ok {
		return text
	}
	return claudeContentParts(content)
}

func claudeContentParts(content interface{}) []interface{} {
	if text, ok := content.(string); ok {
		return []interface{}{map[string]interface{}{"type": "text", "text": text}}
	}
	items, ok := content.([]interface{})
	if !ok {
		return []interface{}{content}
	}
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		part, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, item)
			continue
		}
		switch part["type"] {
		case "text":
			out = append(out, part)
		case "image_url":
			image, _ := part["image_url"].(map[string]interface{})
			out = append(out, map[string]interface{}{"type": "image", "source": claudeImageSource(image)})
		default:
			out = append(out, part)
		}
	}
	return out
}

func claudeImageSource(image map[string]interface{}) map[string]interface{} {
	imageURL, _ := image["url"].(string)
	if strings.HasPrefix(imageURL, "data:") {
		parts := strings.SplitN(imageURL, ",", 2)
		if len(parts) == 2 {
			mediaType := strings.TrimPrefix(strings.SplitN(parts[0], ";", 2)[0], "data:")
			return map[string]interface{}{"type": "base64", "media_type": mediaType, "data": parts[1]}
		}
	}
	return map[string]interface{}{"type": "url", "url": imageURL}
}

func geminiParts(content interface{}) []interface{} {
	if text, ok := content.(string); ok {
		return []interface{}{map[string]interface{}{"text": text}}
	}
	items, ok := content.([]interface{})
	if !ok {
		return []interface{}{map[string]interface{}{"text": fmt.Sprint(content)}}
	}
	parts := make([]interface{}, 0, len(items))
	for _, item := range items {
		part, ok := item.(map[string]interface{})
		if !ok {
			parts = append(parts, map[string]interface{}{"text": fmt.Sprint(item)})
			continue
		}
		switch part["type"] {
		case "text":
			parts = append(parts, map[string]interface{}{"text": part["text"]})
		case "image_url", "video_url":
			key := "image_url"
			if part["type"] == "video_url" {
				key = "video_url"
			}
			media, _ := part[key].(map[string]interface{})
			parts = append(parts, geminiMediaPart(media))
		default:
			if text, ok := part["text"].(string); ok {
				parts = append(parts, map[string]interface{}{"text": text})
			} else {
				parts = append(parts, part)
			}
		}
	}
	return parts
}

func geminiMediaPart(media map[string]interface{}) map[string]interface{} {
	mediaURL, _ := media["url"].(string)
	if strings.HasPrefix(mediaURL, "data:") {
		parts := strings.SplitN(mediaURL, ",", 2)
		if len(parts) == 2 {
			mediaType := strings.TrimPrefix(strings.SplitN(parts[0], ";", 2)[0], "data:")
			if _, err := base64.StdEncoding.DecodeString(parts[1]); err == nil {
				return map[string]interface{}{"inlineData": map[string]interface{}{"mimeType": mediaType, "data": parts[1]}}
			}
		}
	}
	return map[string]interface{}{"fileData": map[string]interface{}{"mimeType": "application/octet-stream", "fileUri": mediaURL}}
}

func geminiTools(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok || len(items) == 0 {
		return nil, false
	}
	declarations := make([]interface{}, 0, len(items))
	for _, item := range items {
		tool, _ := item.(map[string]interface{})
		if tool == nil {
			continue
		}
		if _, alreadyNative := tool["functionDeclarations"]; alreadyNative {
			return items, true
		}
		if fn, ok := tool["function"].(map[string]interface{}); ok {
			declarations = append(declarations, fn)
		}
	}
	if len(declarations) == 0 {
		return nil, false
	}
	return []interface{}{map[string]interface{}{"functionDeclarations": declarations}}, true
}

func copyClaudeOption(target, source map[string]interface{}, key string) {
	if value, ok := source[key]; ok && value != nil {
		target[key] = value
	}
}

func copyGeminiOption(target, source map[string]interface{}, sourceKey, targetKey string) {
	if value, ok := source[sourceKey]; ok && value != nil {
		target[targetKey] = value
	}
}

func firstInt(values map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		switch value := values[key].(type) {
		case int:
			if value > 0 {
				return value
			}
		case int64:
			if value > 0 {
				return int(value)
			}
		case float64:
			if value > 0 {
				return int(value)
			}
		case json.Number:
			if parsed, err := value.Int64(); err == nil && parsed > 0 {
				return int(parsed)
			}
		}
	}
	return 0
}

func decodeChatResponse(protocol string, raw []byte) (*ChatResponse, error) {
	if protocol == chatProtocolOpenAI {
		var result ChatResponse
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	if protocol == chatProtocolClaude {
		var payload struct {
			Content []map[string]interface{} `json:"content"`
			Usage   struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		var builder strings.Builder
		for _, item := range payload.Content {
			if itemType := stringAnyRuntime(item["type"]); itemType == "text" || itemType == "" {
				builder.WriteString(stringAnyRuntime(item["text"]))
			}
		}
		usage := ChatUsage{PromptTokens: payload.Usage.InputTokens, CompletionTokens: payload.Usage.OutputTokens, TotalTokens: payload.Usage.InputTokens + payload.Usage.OutputTokens, CacheReadInputTokens: payload.Usage.CacheReadInputTokens, CacheCreationInputTokens: payload.Usage.CacheCreationInputTokens}
		return &ChatResponse{Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: builder.String()}}}, Usage: usage, ContentBlocks: mapSliceToInterfaces(payload.Content)}, nil
	}
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []map[string]interface{} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Usage struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	var builder strings.Builder
	if len(payload.Candidates) > 0 {
		for _, part := range payload.Candidates[0].Content.Parts {
			builder.WriteString(stringAnyRuntime(part["text"]))
		}
	}
	usage := ChatUsage{PromptTokens: payload.Usage.PromptTokenCount, CompletionTokens: payload.Usage.CandidatesTokenCount, TotalTokens: payload.Usage.TotalTokenCount}
	var blocks []interface{}
	if len(payload.Candidates) > 0 {
		blocks = mapSliceToInterfaces(payload.Candidates[0].Content.Parts)
	}
	return &ChatResponse{Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: builder.String()}}}, Usage: usage, ContentBlocks: blocks}, nil
}

func stringAnyRuntime(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func mapSliceToInterfaces(values []map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func consumeChatStream(reader io.Reader, protocol string, ch chan<- StreamChunk) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	eventName := ""
	usage := ChatUsage{}
	hasUsage := false
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		data := ""
		if strings.HasPrefix(line, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if strings.HasPrefix(line, "{") {
			data = line
		}
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			ch <- StreamChunk{Done: true}
			return
		}
		event, err := decodeChatStreamEvent(protocol, eventName, []byte(data))
		if err != nil {
			ch <- StreamChunk{Error: err}
			return
		}
		if event.Content != "" || event.ReasoningContent != "" {
			ch <- StreamChunk{Content: event.Content, ReasoningContent: event.ReasoningContent}
		}
		if len(event.ToolCalls) > 0 {
			ch <- StreamChunk{ToolCalls: event.ToolCalls}
		}
		if event.Usage != nil {
			mergeChatUsage(&usage, event.Usage)
			hasUsage = true
			current := usage
			ch <- StreamChunk{Usage: &current}
		}
		if event.Done {
			ch <- StreamChunk{Done: true}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Error: err}
		return
	}
	_ = hasUsage
	ch <- StreamChunk{Done: true}
}

type decodedChatStreamEvent struct {
	Content          string
	ReasoningContent string
	ToolCalls        []map[string]interface{}
	Usage            *ChatUsage
	Done             bool
}

func decodeChatStreamEvent(protocol, eventName string, raw []byte) (decodedChatStreamEvent, error) {
	if protocol == chatProtocolOpenAI {
		var event struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *ChatUsage `json:"usage"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return decodedChatStreamEvent{}, nil
		}
		result := decodedChatStreamEvent{Usage: event.Usage}
		if len(event.Choices) > 0 {
			result.Content = event.Choices[0].Delta.Content
			result.ReasoningContent = event.Choices[0].Delta.ReasoningContent
		}
		if len(event.Choices) > 0 {
			for _, call := range event.Choices[0].Delta.ToolCalls {
				result.ToolCalls = append(result.ToolCalls, map[string]interface{}{"index": call.Index, "id": call.ID, "function": map[string]interface{}{"name": call.Function.Name, "arguments": call.Function.Arguments}})
			}
		}
		return result, nil
	}
	if protocol == chatProtocolClaude {
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Text        string `json:"text"`
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return decodedChatStreamEvent{}, err
		}
		eventType := eventName
		if eventType == "" {
			eventType = event.Type
		}
		switch eventType {
		case "message_start":
			return decodedChatStreamEvent{Usage: &ChatUsage{PromptTokens: event.Message.Usage.InputTokens}}, nil
		case "message_delta":
			return decodedChatStreamEvent{Usage: &ChatUsage{CompletionTokens: event.Usage.OutputTokens}}, nil
		case "content_block_delta":
			if event.Delta.Type == "input_json_delta" || event.Delta.PartialJSON != "" {
				return decodedChatStreamEvent{ToolCalls: []map[string]interface{}{{"type": "input_json_delta", "partial_json": event.Delta.PartialJSON}}}, nil
			}
			return decodedChatStreamEvent{Content: event.Delta.Text}, nil
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				return decodedChatStreamEvent{ToolCalls: []map[string]interface{}{{"type": "tool_use", "id": event.ContentBlock.ID, "name": event.ContentBlock.Name}}}, nil
			}
			return decodedChatStreamEvent{}, nil
		case "message_stop":
			return decodedChatStreamEvent{Done: true}, nil
		case "error":
			return decodedChatStreamEvent{}, errors.New(connectionTestMessage(raw))
		default:
			return decodedChatStreamEvent{}, nil
		}
	}
	var event struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string                 `json:"text"`
					FunctionCall map[string]interface{} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		Usage struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return decodedChatStreamEvent{}, err
	}
	var builder strings.Builder
	calls := make([]map[string]interface{}, 0)
	done := false
	if len(event.Candidates) > 0 {
		for _, part := range event.Candidates[0].Content.Parts {
			builder.WriteString(part.Text)
			if len(part.FunctionCall) > 0 {
				calls = append(calls, map[string]interface{}{"functionCall": part.FunctionCall})
			}
		}
		done = event.Candidates[0].FinishReason != ""
	}
	usage := &ChatUsage{PromptTokens: event.Usage.PromptTokenCount, CompletionTokens: event.Usage.CandidatesTokenCount, TotalTokens: event.Usage.TotalTokenCount}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		usage = nil
	}
	return decodedChatStreamEvent{Content: builder.String(), ToolCalls: calls, Usage: usage, Done: done}, nil
}

func mergeChatUsage(target *ChatUsage, source *ChatUsage) {
	if source == nil {
		return
	}
	if source.PromptTokens > 0 {
		target.PromptTokens = source.PromptTokens
	}
	if source.CompletionTokens > 0 {
		target.CompletionTokens = source.CompletionTokens
	}
	if source.TotalTokens > 0 {
		target.TotalTokens = source.TotalTokens
	}
	if source.CacheReadInputTokens > 0 {
		target.CacheReadInputTokens = source.CacheReadInputTokens
	}
	if source.CacheCreationInputTokens > 0 {
		target.CacheCreationInputTokens = source.CacheCreationInputTokens
	}
	if source.TotalTokens == 0 && (source.PromptTokens > 0 || source.CompletionTokens > 0) {
		target.TotalTokens = target.PromptTokens + target.CompletionTokens
	} else if target.TotalTokens == 0 {
		target.TotalTokens = target.PromptTokens + target.CompletionTokens
	}
}
