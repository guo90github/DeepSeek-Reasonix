package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProjectConfigCannotOverrideSecrets pins [secrets] as a user-global
// security control: a cloned repository's reasonix.toml must not be able to
// opt the user into subprocess env stripping or sensitive-path hiding.
func TestProjectConfigCannotOverrideSecrets(t *testing.T) {
	isolateUserConfigHome(t)
	t.Setenv("REASONIX_HOME", "")
	globalDir := filepath.Dir(UserConfigPath())
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalTOML := "[secrets]\nfilter_subprocess_env = false\nprotect_sensitive_files = false\n"
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"), []byte(globalTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	projectTOML := "[secrets]\nfilter_subprocess_env = true\nprotect_sensitive_files = true\n"
	if err := os.WriteFile(filepath.Join(project, "reasonix.toml"), []byte(projectTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadForRoot(project)
	if err != nil {
		t.Fatalf("LoadForRoot() error = %v", err)
	}
	if cfg.Secrets.FilterSubprocessEnv {
		t.Error("project reasonix.toml enabled filter_subprocess_env; [secrets] must stay user-global")
	}
	if cfg.Secrets.ProtectSensitiveFiles {
		t.Error("project reasonix.toml enabled protect_sensitive_files; [secrets] must stay user-global")
	}
}

// TestRenderTOMLPersistsSecretsSection pins config-save round-tripping: the
// renderer must emit [secrets] for the user scope or every WriteFile would
// silently drop the user's security toggles.
func TestRenderTOMLPersistsSecretsSection(t *testing.T) {
	cfg := Default()
	cfg.Secrets.FilterSubprocessEnv = true
	cfg.Secrets.ProtectSensitiveFiles = true

	out := RenderTOMLForScope(cfg, RenderScopeUser)
	for _, want := range []string{"[secrets]", "filter_subprocess_env = true", "protect_sensitive_files = true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("user-scope render missing %q:\n%s", want, out)
		}
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	back := Default()
	if err := mergeFile(back, path); err != nil {
		t.Fatalf("round-trip decode: %v", err)
	}
	if !back.Secrets.FilterSubprocessEnv || !back.Secrets.ProtectSensitiveFiles {
		t.Fatalf("secrets toggles lost in render round-trip: %+v", back.Secrets)
	}

	// Project scope must not render the section — LoadForRoot ignores it there.
	if proj := RenderTOMLForScope(cfg, RenderScopeProject); strings.Contains(proj, "[secrets]") {
		t.Fatalf("project scope rendered [secrets]:\n%s", proj)
	}
	if strings.Contains(out, "redact_tool_output") {
		t.Fatalf("user-scope render still exposes removed live-redaction setting:\n%s", out)
	}
}
