package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestSetPromptOptimizeModelCanonicalizesRefAndRejectsAuto(t *testing.T) {
	c := &Config{Providers: []ProviderEntry{{
		Name: "gateway", Kind: "openai", BaseURL: "http://127.0.0.1:1",
		Models: []string{"chat", "other"}, Default: "chat",
	}}}
	if err := c.SetPromptOptimizeModel("gateway/chat"); err != nil || c.Agent.PromptOptimizeModel != "gateway/chat" {
		t.Fatalf("explicit: err=%v value=%q", err, c.Agent.PromptOptimizeModel)
	}
	if err := c.SetPromptOptimizeModel(""); err != nil || c.Agent.PromptOptimizeModel != "" {
		t.Fatalf("clear: err=%v value=%q", err, c.Agent.PromptOptimizeModel)
	}
	if err := c.SetPromptOptimizeModel("auto"); err == nil {
		t.Fatal("auto was accepted — the utility must never track the session model")
	}
	if err := c.SetPromptOptimizeModel("gateway/missing"); err == nil {
		t.Fatal("unknown model was accepted")
	}
}

func TestRemoveProviderClearsPromptOptimizeModel(t *testing.T) {
	c := &Config{Providers: []ProviderEntry{
		{Name: "text", Kind: "openai", BaseURL: "https://text.invalid", Model: "chat"},
		{Name: "opt", Kind: "openai", BaseURL: "https://opt.invalid", Model: "qwen"},
	}, DefaultModel: "text", Agent: AgentConfig{PromptOptimizeModel: "opt/qwen"}}
	if err := c.RemoveProvider("opt"); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	if c.Agent.PromptOptimizeModel != "" {
		t.Fatalf("prompt optimize model = %q, want cleared", c.Agent.PromptOptimizeModel)
	}
}

func TestPromptOptimizeModelRoundTripsThroughTOML(t *testing.T) {
	c := Default()
	c.Agent.PromptOptimizeModel = "deepseek-pro/deepseek-chat"
	var decoded Config
	if _, err := toml.Decode(RenderTOML(c), &decoded); err != nil {
		t.Fatalf("decode rendered config: %v", err)
	}
	if decoded.Agent.PromptOptimizeModel != "deepseek-pro/deepseek-chat" {
		t.Fatalf("prompt_optimize_model = %q, want deepseek-pro/deepseek-chat", decoded.Agent.PromptOptimizeModel)
	}
}
