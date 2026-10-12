package config

import (
	"strings"
	"testing"
	"time"
)

// TestRenderedConfigNamesTheAskTimeout keeps the switch discoverable for someone
// reading their own config.toml: the sample names the key and the two non-default
// polarities, so neither the fallback nor "wait for me" has to be guessed.
func TestRenderedConfigNamesTheAskTimeout(t *testing.T) {
	for _, scope := range []RenderScope{RenderScopeUser, RenderScopeProject} {
		body := RenderTOMLForScope(Default(), scope)
		if !strings.Contains(body, "# ask_timeout_minutes = ") {
			t.Fatalf("the rendered %v sample never names the ask timeout:\n%s", scope, body)
		}
		if !strings.Contains(body, "negative waits for the user") {
			t.Fatalf("the rendered %v sample does not explain the negative value:\n%s", scope, body)
		}
	}
}

func TestRenderedAskTimeoutCarriesTheConfiguredValue(t *testing.T) {
	cfg := Default()
	cfg.Agent.AskTimeoutMinutes = 2.5
	body := RenderTOMLForScope(cfg, RenderScopeUser)
	if !strings.Contains(body, "ask_timeout_minutes = 2.5") {
		t.Fatalf("the rendered sample dropped the configured value:\n%s", body)
	}
	if got := cfg.AskTimeout(); got != 150*time.Second {
		t.Fatalf("configured 2.5 minutes rendered as %v", got)
	}
}
