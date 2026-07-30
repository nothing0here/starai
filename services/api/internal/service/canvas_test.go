package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateCanvasInput(t *testing.T) {
	valid := SaveCanvasInput{
		Title: "  商品图流程  ",
		Document: json.RawMessage(`{
			"version": 1,
			"nodes": [{"id":"a"},{"id":"b"}],
			"edges": [{"id":"a-b","source":"a","target":"b"}],
			"viewport": {"x":0,"y":0,"zoom":1}
		}`),
	}
	if err := validateCanvasInput(&valid); err != nil {
		t.Fatalf("expected valid canvas, got %v", err)
	}
	if valid.Title != "商品图流程" {
		t.Fatalf("expected trimmed title, got %q", valid.Title)
	}

	invalidJSON := SaveCanvasInput{Document: json.RawMessage(`{"nodes":`)}
	if err := validateCanvasInput(&invalidJSON); err == nil {
		t.Fatal("expected invalid JSON error")
	}

	tooManyNodes := SaveCanvasInput{
		Document: json.RawMessage(`{"version":1,"nodes":[` + strings.Repeat(`{},`, 500) + `{}` + `],"edges":[]}`),
	}
	if err := validateCanvasInput(&tooManyNodes); err == nil {
		t.Fatal("expected node limit error")
	}
}
