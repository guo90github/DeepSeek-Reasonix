package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveToKeepsTheShellAsyncTier pins the write path the desktop tier switch
// uses. It renders the whole user config, so a key the renderer never prints is
// dropped on the next save — and a dropped tier reads back as the default off.
func TestSaveToKeepsTheShellAsyncTier(t *testing.T) {
	isolateUserConfigHome(t)
	userPath := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}

	c := Default()
	if err := c.SetAgentShellAsync("fast"); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveTo(userPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	body, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `shell_async = "fast"`) {
		t.Errorf("the saved user config does not carry the tier:\n%s", body)
	}
	back := LoadForEditWithoutCredentials(userPath)
	if back.Agent.ShellAsync != "fast" {
		t.Fatalf("shell_async = %q after a save, want %q", back.Agent.ShellAsync, "fast")
	}
}

// TestRenderedConfigNamesTheShellAsyncTier keeps the switch discoverable for
// someone reading their own config.toml — user and project samples alike, since
// both go through the same scope-aware renderer.
func TestRenderedConfigNamesTheShellAsyncTier(t *testing.T) {
	for _, scope := range []RenderScope{RenderScopeUser, RenderScopeProject} {
		body := RenderTOMLForScope(Default(), scope)
		if !strings.Contains(body, "# shell_async = ") {
			t.Fatalf("the rendered %v sample never names the shell_async switch:\n%s", scope, body)
		}
	}
}
