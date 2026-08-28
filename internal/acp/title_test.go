package acp

import (
	"testing"

	"reasonix/internal/provider"
)

func TestTitleFromHistorySkipsSyntheticUserNudges(t *testing.T) {
	history := []provider.Message{
		// First user-role message is a host-injected finish-protocol repair
		// (persisted shape with transient preference blocks) — not a title.
		{Role: provider.RoleUser, Content: "<reasoning-language>\nzh\n</reasoning-language>\n\nProtocol repair: finish this turn now. A visible final answer has already been provided, so do not repeat it."},
		{Role: provider.RoleUser, Content: "fix the login bug"},
	}
	if got := titleFromHistory(history); got != "fix the login bug" {
		t.Fatalf("titleFromHistory() = %q, want the authored user prompt", got)
	}
}
