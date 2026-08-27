package config

import "testing"

func TestLoadForRoot_ParsesOptimizeSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)

	project := t.TempDir()
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", `
[optimize]
base_url = "https://example.com/v1"
api_key  = "sk-secret"
model    = "qwen3.7-plus"
max_tokens = 512
timeout_ms  = 15000
include_history = true
history_turns = 4
`)
	cfg, err := LoadForRoot(project)
	if err != nil {
		t.Fatalf("LoadForRoot: %v", err)
	}
	opt := cfg.Optimize
	if opt.BaseURL != "https://example.com/v1" || opt.APIKey != "sk-secret" || opt.Model != "qwen3.7-plus" {
		t.Fatalf("Optimize = %+v, want parsed base_url/api_key/model", opt)
	}
	if opt.MaxTokens != 512 || opt.TimeoutMS != 15000 {
		t.Fatalf("Optimize = %+v, want max_tokens=512 timeout_ms=15000", opt)
	}
	if !opt.IncludeHistory {
		t.Fatalf("IncludeHistory = false, want parsed true")
	}
	if opt.HistoryTurns != 4 {
		t.Fatalf("HistoryTurns = %d, want parsed 4", opt.HistoryTurns)
	}
	if !cfg.OptimizeEnabled() {
		t.Fatalf("OptimizeEnabled() = false, want true with full [optimize] config")
	}
}

func TestOptimizeEnabledRequiresAllFields(t *testing.T) {
	cfg := Default()
	cfg.Optimize.BaseURL = "https://example.com/v1"
	if cfg.OptimizeEnabled() {
		t.Fatalf("OptimizeEnabled() = true with only base_url set")
	}
	cfg.Optimize.APIKey = "k"
	cfg.Optimize.Model = "m"
	if !cfg.OptimizeEnabled() {
		t.Fatalf("OptimizeEnabled() = false with all fields set")
	}
}

func TestDefaultOptimizeModel(t *testing.T) {
	cfg := Default()
	if cfg.Optimize.Model != defaultOptimizeModel {
		t.Fatalf("default optimize model = %q, want %q", cfg.Optimize.Model, defaultOptimizeModel)
	}
	if cfg.Optimize.IncludeHistory {
		t.Fatalf("IncludeHistory must default to false (sends recent conversation to a possibly different provider)")
	}
}
