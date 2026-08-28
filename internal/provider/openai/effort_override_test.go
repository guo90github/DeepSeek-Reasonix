package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func newTestClient(t *testing.T, model string, extra map[string]any) *client {
	t.Helper()
	cfg := provider.Config{Name: "p", BaseURL: "https://api.deepseek.com", Model: model, APIKey: "k", Extra: extra}
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p.(*client)
}

func TestEffortOverrideDeepSeekFlash(t *testing.T) {
	c := newTestClient(t, "deepseek-v4-flash", map[string]any{"reasoning_protocol": "deepseek"})
	if got := c.buildRequest(provider.Request{}).ReasoningEffort; got != "high" {
		t.Fatalf("default reasoning_effort = %q, want high", got)
	}
	if got := c.buildRequest(provider.Request{EffortOverride: "low"}).ReasoningEffort; got != "low" {
		t.Fatalf("override low: reasoning_effort = %q, want low", got)
	}
	if got := c.buildRequest(provider.Request{EffortOverride: "medium"}).ReasoningEffort; got != "high" {
		t.Fatalf("override outside DeepSeek vocabulary must keep the default, got %q", got)
	}
	off := c.buildRequest(provider.Request{EffortOverride: "disabled"})
	if off.Thinking == nil || off.Thinking.Type != "disabled" || off.ReasoningEffort != "" {
		t.Fatalf("override disabled = thinking %+v, effort %q; want disabled/empty", off.Thinking, off.ReasoningEffort)
	}
}

func TestEffortOverrideDeepSeekNonFlashRejectsLow(t *testing.T) {
	c := newTestClient(t, "deepseek-v4", map[string]any{"reasoning_protocol": "deepseek"})
	if got := c.buildRequest(provider.Request{EffortOverride: "low"}).ReasoningEffort; got != "high" {
		t.Fatalf("low is flash-only; reasoning_effort = %q, want high", got)
	}
	if got := c.buildRequest(provider.Request{EffortOverride: "max"}).ReasoningEffort; got != "max" {
		t.Fatalf("max is in the official DeepSeek vocabulary, got %q", got)
	}
}

func TestEffortOverrideDeepSeekProSupportsLow(t *testing.T) {
	c := newTestClient(t, "deepseek-v4-pro", map[string]any{"reasoning_protocol": "deepseek"})
	if got := c.buildRequest(provider.Request{EffortOverride: "low"}).ReasoningEffort; got != "low" {
		t.Fatalf("Pro low reasoning_effort = %q, want low", got)
	}
}

func TestEffortOverrideHonorsSupportedEfforts(t *testing.T) {
	c := newTestClient(t, "deepseek-v4", map[string]any{
		"reasoning_protocol": "deepseek",
		"effort":             "high",
		"supported_efforts":  []string{"low", "high", "disabled"},
	})
	if got := c.buildRequest(provider.Request{EffortOverride: "low"}).ReasoningEffort; got != "low" {
		t.Fatalf("declared vocabulary must admit low, got %q", got)
	}
	if got := c.buildRequest(provider.Request{EffortOverride: "max"}).ReasoningEffort; got != "high" {
		t.Fatalf("undeclared level must keep the default, got %q", got)
	}
	off := c.buildRequest(provider.Request{EffortOverride: "disabled"})
	if off.Thinking == nil || off.Thinking.Type != "disabled" || off.ReasoningEffort != "" {
		t.Fatalf("override disabled = thinking %+v, effort %q; want disabled/empty", off.Thinking, off.ReasoningEffort)
	}
}

func TestEffortOverrideDisabledTurnsThinkingOff(t *testing.T) {
	c := newTestClient(t, "deepseek-v4", map[string]any{"reasoning_protocol": "deepseek"})
	off := c.buildRequest(provider.Request{EffortOverride: "disabled"})
	if off.Thinking == nil || off.Thinking.Type != "disabled" || off.ReasoningEffort != "" {
		t.Fatalf("override disabled = thinking %+v, effort %q; want disabled/empty", off.Thinking, off.ReasoningEffort)
	}
	// The session path never sets the override: configured thinking and depth stay.
	normal := c.buildRequest(provider.Request{})
	if normal.Thinking == nil || normal.Thinking.Type != "enabled" || normal.ReasoningEffort != "high" {
		t.Fatalf("no override must keep thinking %+v / effort %q, want enabled/high", normal.Thinking, normal.ReasoningEffort)
	}
}

func TestEffortOverrideDisabledQwenDropsEnableThinking(t *testing.T) {
	p, err := New(provider.Config{
		Name:    "qwen",
		BaseURL: "https://ws-x.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		Model:   "qwen3.7-plus",
		APIKey:  "k",
		Extra:   map[string]any{"extra_body": map[string]any{"enable_thinking": true}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := p.(*client)
	off := marshalChatRequest(t, c.buildRequest(provider.Request{EffortOverride: "disabled"}))
	if got, _ := off["enable_thinking"].(bool); got {
		t.Fatalf("override disabled must send enable_thinking=false, got %v", off["enable_thinking"])
	}
	normal := marshalChatRequest(t, c.buildRequest(provider.Request{}))
	if got, _ := normal["enable_thinking"].(bool); !got {
		t.Fatalf("no override must keep the provider's enable_thinking=true, got %v", normal["enable_thinking"])
	}
}

func marshalChatRequest(t *testing.T, r chatRequest) map[string]any {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return body
}

func TestEffortOverrideIgnoredByBinaryThinkingKnobs(t *testing.T) {
	for _, c := range []*client{
		{model: "MiniMax-M3", minimax: true},
		{model: "glm-4.5-air", zhipu: true},
		{model: "LongCat-Flash", longcat: true},
	} {
		out := c.buildRequest(provider.Request{EffortOverride: "low"})
		if out.ReasoningEffort != "" {
			t.Errorf("%s: reasoning_effort = %q, want omitted", c.model, out.ReasoningEffort)
		}
		if out.Thinking != nil && strings.Contains(out.Thinking.Type, "low") {
			t.Errorf("%s: override leaked into thinking.type %q", c.model, out.Thinking.Type)
		}
	}
}

func TestEffortOverrideIgnoredWithoutDepthVocabulary(t *testing.T) {
	// A generic gateway with no configured effort and no supported_efforts has
	// no evidence of a depth scale — the override must not invent wire fields.
	c := &client{model: "mimo-v2"}
	if got := c.buildRequest(provider.Request{EffortOverride: "low"}).ReasoningEffort; got != "" {
		t.Fatalf("reasoning_effort = %q, want omitted", got)
	}

	disabled := newTestClient(t, "deepseek-v4", map[string]any{"reasoning_protocol": "deepseek", "thinking": "disabled"})
	if got := disabled.buildRequest(provider.Request{EffortOverride: "high"}).ReasoningEffort; got != "" {
		t.Fatalf("thinking-disabled provider must ignore overrides, got %q", got)
	}
}
