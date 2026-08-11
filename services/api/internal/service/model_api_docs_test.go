package service

import "testing"

func TestStandardAPIDocContentNormalizesLegacyChatResponse(t *testing.T) {
	doc := &APIDocDTO{Slug: "chat-model", ModelCode: "chat-model", RequestMode: "chat_completions", Endpoint: "/v1/chat/completions"}
	content := standardAPIDocContent(doc, map[string]interface{}{
		"response_example": map[string]interface{}{"code": 0, "message": "ok", "data": map[string]interface{}{}},
		"responses": map[string]interface{}{
			"200": map[string]interface{}{"description": "请求成功", "body": map[string]interface{}{"code": 0, "data": map[string]interface{}{}}},
		},
	})

	response, ok := content["response_example"].(map[string]interface{})
	if !ok || response["object"] != "chat.completion" {
		t.Fatalf("expected native chat completion response, got %#v", content["response_example"])
	}
	parameters, ok := content["parameters"].([]map[string]interface{})
	if !ok || len(parameters) == 0 {
		t.Fatalf("expected standard chat parameters, got %#v", content["parameters"])
	}
	responses, ok := content["responses"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response definitions, got %#v", content["responses"])
	}
	success, ok := responses["200"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 200 response definition, got %#v", responses["200"])
	}
	successBody, ok := success["body"].(map[string]interface{})
	if !ok || successBody["object"] != "chat.completion" {
		t.Fatalf("expected native 200 response body, got %#v", success["body"])
	}
}

func TestStandardAPIDocContentUsesNativeMediaTaskResponse(t *testing.T) {
	doc := &APIDocDTO{Slug: "image-model", ModelCode: "image-model", RequestMode: "images", Endpoint: "/v1/images/generations"}
	content := standardAPIDocContent(doc, nil)
	response, ok := content["response_example"].(map[string]interface{})
	if !ok || response["task_no"] != "task_xxx" || response["status"] != "pending" {
		t.Fatalf("expected native media task response, got %#v", content["response_example"])
	}
	if _, wrapped := response["data"]; wrapped {
		t.Fatalf("media task response must not use legacy data envelope: %#v", response)
	}
}

func TestStandardAPIDocResponsesUseNativeErrorShape(t *testing.T) {
	responses := standardAPIDocResponses(map[string]interface{}{
		"responses": map[string]interface{}{
			"400": map[string]interface{}{"description": "请求参数错误", "body": map[string]interface{}{"code": 400, "message": "参数错误"}},
		},
	}, map[string]interface{}{"object": "chat.completion"})
	badRequest, ok := responses["400"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 400 response definition, got %#v", responses["400"])
	}
	body, ok := badRequest["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 400 body, got %#v", badRequest["body"])
	}
	if _, ok := body["error"].(map[string]interface{}); !ok {
		t.Fatalf("expected native error body, got %#v", body)
	}
}
