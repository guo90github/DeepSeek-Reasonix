package config

import (
	"strings"
	"testing"
)

func TestSetAgentShellAsyncAcceptsOnlyKnownTiers(t *testing.T) {
	c := Default()
	for _, tier := range []string{"off", "balanced", "fast", "BALANCED", " fast "} {
		if err := c.SetAgentShellAsync(tier); err != nil {
			t.Fatalf("SetAgentShellAsync(%q): %v", tier, err)
		}
		if got := c.Agent.ShellAsync; got != strings.ToLower(strings.TrimSpace(tier)) {
			t.Fatalf("ShellAsync after %q = %q", tier, got)
		}
	}
	if err := c.SetAgentShellAsync("turbo"); err == nil {
		t.Fatal("an unknown tier must be refused: it decides when a command runs in the background")
	}
	if got := c.Agent.ShellAsync; got != "fast" {
		t.Fatalf("a refused tier changed the stored value to %q", got)
	}
}
