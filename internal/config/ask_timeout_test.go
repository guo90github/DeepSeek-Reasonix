package config

import (
	"testing"
	"time"
)

func TestAskTimeoutResolvesTheKnob(t *testing.T) {
	cfg := &Config{}
	if got := cfg.AskTimeout(); got != 0 {
		t.Fatalf("unset ask_timeout_minutes = %v, want 0 so control keeps its default", got)
	}
	cfg.Agent.AskTimeoutMinutes = 2.5
	if got := cfg.AskTimeout(); got != 150*time.Second {
		t.Fatalf("ask_timeout_minutes = 2.5 resolved to %v, want 2m30s", got)
	}
	cfg.Agent.AskTimeoutMinutes = -1
	if got := cfg.AskTimeout(); got >= 0 {
		t.Fatalf("negative ask_timeout_minutes = %v, want a negative value that disables the fallback", got)
	}
	var missing *Config
	if got := missing.AskTimeout(); got != 0 {
		t.Fatalf("nil config = %v, want 0", got)
	}
}
