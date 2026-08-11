package handler

import "testing"

func TestAnthropicCompletionInputPreservesTextAndMedia(t *testing.T) {
	input, err := anthropicCompletionInput(map[string]interface{}{
		"model":  "claude-native",
		"system": "You are concise.",
		"messages": []interface{}{map[string]interface{}{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "Describe this image"},
				map[string]interface{}{"type": "image", "source": map[string]interface{}{"type": "url", "url": "https://example.com/cat.png"}},
			},
		}},
		"max_tokens": 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Messages) != 2 || input.Messages[1].Role != "user" || input.Messages[1].Content != "Describe this image" {
		t.Fatalf("unexpected messages: %#v", input.Messages)
	}
	images, ok := input.Params["reference_images"].([]string)
	if !ok || len(images) != 1 || images[0] != "https://example.com/cat.png" {
		t.Fatalf("unexpected reference images: %#v", input.Params["reference_images"])
	}
}

func TestGeminiCompletionInputMapsNativeParts(t *testing.T) {
	input, err := geminiCompletionInput("gemini-native", map[string]interface{}{
		"systemInstruction": map[string]interface{}{"parts": []interface{}{map[string]interface{}{"text": "Be concise"}}},
		"contents": []interface{}{map[string]interface{}{
			"role":  "user",
			"parts": []interface{}{map[string]interface{}{"text": "Hello"}, map[string]interface{}{"inline_data": map[string]interface{}{"mime_type": "image/png", "data": "abc"}}},
		}},
		"generationConfig": map[string]interface{}{"maxOutputTokens": float64(128), "topP": 0.8},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Messages) != 2 || input.Messages[0].Role != "system" || input.Messages[1].Content != "Hello" {
		t.Fatalf("unexpected messages: %#v", input.Messages)
	}
	if input.Params["max_tokens"] != float64(128) || input.Params["top_p"] != 0.8 {
		t.Fatalf("unexpected generation params: %#v", input.Params)
	}
	images, ok := input.Params["reference_images"].([]string)
	if !ok || len(images) != 1 || images[0] != "data:image/png;base64,abc" {
		t.Fatalf("unexpected inline image: %#v", input.Params["reference_images"])
	}
}
