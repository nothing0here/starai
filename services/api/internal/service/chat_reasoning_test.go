package service

import "testing"

func TestBuildChatUpstreamParamsEnablesNVIDIAReasoningWithDefaultBudget(t *testing.T) {
	model := &ModelFull{
		RuntimeRule: map[string]interface{}{
			"reasoning": map[string]interface{}{
				"mode":           "nvidia_chat_template",
				"default_budget": 16384,
				"max_budget":     32768,
			},
		},
	}

	params, err := buildChatUpstreamParams(model, map[string]interface{}{"deep_think": true})
	if err != nil {
		t.Fatal(err)
	}
	template, ok := params["chat_template_kwargs"].(map[string]interface{})
	if !ok || template["enable_thinking"] != true {
		t.Fatalf("chat_template_kwargs=%#v", params["chat_template_kwargs"])
	}
	if params["reasoning_budget"] != 16384 {
		t.Fatalf("reasoning_budget=%#v", params["reasoning_budget"])
	}
}

func TestBuildChatUpstreamParamsRejectsBudgetOverModelLimit(t *testing.T) {
	model := &ModelFull{RuntimeRule: map[string]interface{}{
		"reasoning": map[string]interface{}{
			"mode":       "nvidia_chat_template",
			"max_budget": 16384,
		},
	}}

	_, err := buildChatUpstreamParams(model, map[string]interface{}{
		"deep_think":       true,
		"reasoning_budget": 16385,
	})
	if err == nil {
		t.Fatal("expected an error for a budget over the model limit")
	}
}

func TestBuildChatUpstreamParamsLeavesModelsWithoutReasoningMappingUntouched(t *testing.T) {
	params, err := buildChatUpstreamParams(&ModelFull{}, map[string]interface{}{
		"deep_think":       true,
		"reasoning_budget": 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := params["chat_template_kwargs"]; ok {
		t.Fatalf("unexpected NVIDIA parameters: %#v", params)
	}
	if _, ok := params["reasoning_budget"]; ok {
		t.Fatalf("unexpected reasoning budget: %#v", params)
	}
}
