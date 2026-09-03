package service

import (
	"strings"
	"testing"
)

func TestAgentPolicySectionsAreEffectiveAndBackwardCompatible(t *testing.T) {
	p := AgentPolicyFromConfig(map[string]interface{}{"agent_policy": map[string]interface{}{"version": 3, "instructions": "自定义品牌要求"}})
	if p.Version != 3 || p.Instructions != "自定义品牌要求" || p.IntentGuidance == "" || p.CreationGuidance == "" {
		t.Fatalf("legacy policy lost defaults: %#v", p)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.ResearchGuidance = "自定义来源核验要求"
	if !strings.Contains(p.Prompt(), "自定义来源核验要求") || !strings.Contains(p.Prompt(), "自定义品牌要求") {
		t.Fatal("editor values do not reach prompt")
	}
	p.CreationGuidance = strings.Repeat("字", 6001)
	if p.Validate() == nil {
		t.Fatal("unbounded policy accepted")
	}
}
