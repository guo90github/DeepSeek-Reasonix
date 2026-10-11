package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveToKeepsThePlanExecuteTier pins the write path: the renderer has to
// print the key, because a key it never prints is dropped on the next save — and
// a dropped tier reads back as the default off, silently disabling the tool.
func TestSaveToKeepsThePlanExecuteTier(t *testing.T) {
	isolateUserConfigHome(t)
	userPath := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}

	c := Default()
	c.Agent.PlanExecute = "on"
	if err := c.SaveTo(userPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	body, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `plan_execute = "on"`) {
		t.Errorf("the saved user config does not carry the tier:\n%s", body)
	}
	back := LoadForEditWithoutCredentials(userPath)
	if back.Agent.PlanExecute != "on" {
		t.Fatalf("plan_execute = %q after a save, want %q", back.Agent.PlanExecute, "on")
	}
}

// TestRenderedConfigNamesThePlanExecuteTier keeps the switch discoverable for
// someone reading their own config.toml — user and project samples alike.
func TestRenderedConfigNamesThePlanExecuteTier(t *testing.T) {
	for _, scope := range []RenderScope{RenderScopeUser, RenderScopeProject} {
		body := RenderTOMLForScope(Default(), scope)
		if !strings.Contains(body, "# plan_execute = ") {
			t.Fatalf("the rendered %v sample never names the plan_execute switch:\n%s", scope, body)
		}
	}
}
