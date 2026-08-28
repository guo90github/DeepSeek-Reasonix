package responses

import (
	"testing"

	"reasonix/internal/provider"
)

func TestRequestEffortOverrideDisabledTurnsOffReasoning(t *testing.T) {
	client := New(Config{Name: "deepseek", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash", Effort: "high"}).(*client)
	body, _, _ := client.buildRequestBody(provider.Request{
		Messages:       []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		EffortOverride: "disabled",
	})
	reasoning, _ := body["reasoning"].(map[string]any)
	if got, _ := reasoning["effort"].(string); got != "none" {
		t.Fatalf("override disabled reasoning.effort = %q, want none", got)
	}
	normal, _, _ := client.buildRequestBody(provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})
	normalReasoning, _ := normal["reasoning"].(map[string]any)
	if got, _ := normalReasoning["effort"].(string); got != "high" {
		t.Fatalf("no override must keep the configured effort, got %q", got)
	}
}
