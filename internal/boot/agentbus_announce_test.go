package boot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// A host that serves its own sessions announces where they speak from, so another host
// can wake them: the seam is Options, and it must reach the board's directory for real.
func TestBootAgentBusHostAnnouncesItsAddress(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	boardDir := filepath.Join(t.TempDir(), "agentbus", "default")
	tokenFile := filepath.Join(t.TempDir(), "serve.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("token file: %v", err)
	}
	rec := &effectRecordingProvider{}
	provider.Register("boot-agentbus-announce", func(provider.Config) (provider.Provider, error) { return rec, nil })

	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-agentbus-announce"
model = "x"
`)
	ctrl, err := Build(context.Background(), Options{
		Sink:              event.Discard,
		AgentBusDir:       boardDir,
		AgentBusID:        "alice",
		AgentBusHost:      "http://127.0.0.1:8977",
		AgentBusTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	ref, ok, err := directory.Lookup("alice")
	if err != nil || !ok {
		t.Fatalf("lookup = (%+v, %v, %v), want the built session's announcement", ref, ok, err)
	}
	if ref.Host != "http://127.0.0.1:8977" || ref.TokenFile != tokenFile {
		t.Fatalf("ref = %+v, want the host and token file the caller passed", ref)
	}
	if ref.SessionPath != ctrl.SessionPath() {
		t.Fatalf("sessionPath = %q, want the built session's own path %q", ref.SessionPath, ctrl.SessionPath())
	}
}

// A host with no endpoint (the desktop before it runs a serve) must not claim one.
func TestBootAgentBusWithoutAHostAnnouncesNothing(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	boardDir := filepath.Join(t.TempDir(), "agentbus", "default")
	rec2 := &effectRecordingProvider{}
	provider.Register("boot-agentbus-announce-off", func(provider.Config) (provider.Provider, error) { return rec2, nil })

	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-agentbus-announce-off"
model = "x"
`)
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard, AgentBusDir: boardDir, AgentBusID: "alice"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	if _, ok, err := directory.Lookup("alice"); err != nil || ok {
		t.Fatalf("lookup = (%v, %v), want no address for a host that serves nothing", ok, err)
	}
}
