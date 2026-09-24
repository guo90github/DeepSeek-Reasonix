package boot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/plugin"
)

// TestBuildResolvesMCPProcessEnvFromTheControllerSession asserts the effect at
// the boundary that matters: the values a frontend resolves reach the stdio
// child's own environment, and the resolver is asked about the controller's own
// session — never the workspace root, which a shared host would have to guess from.
func TestBuildResolvesMCPProcessEnvFromTheControllerSession(t *testing.T) {
	isolateConfigHome(t)
	workspace := robustTempDir(t)
	t.Chdir(workspace)
	out := filepath.Join(t.TempDir(), "child-env.txt")

	var resolved []string
	ctrl, err := Build(context.Background(), Options{
		SessionDir: filepath.Join(t.TempDir(), "sessions"),
		Sink:       event.Discard,
		MCPProcessEnv: func(sessionPath string) map[string]string {
			resolved = append(resolved, sessionPath)
			env := map[string]string{
				"REASONIX_SERVE_URL": "http://127.0.0.1:8787",
				// Proof of what the host decided, even while no session is pinned.
				"REASONIX_RESOLVED_SESSION": sessionPath,
			}
			if strings.TrimSpace(sessionPath) != "" {
				env["REASONIX_SESSION_PATH"] = sessionPath
			}
			return env
		},
		ExtraPlugins: []plugin.Spec{{
			Name:    "env-probe",
			Command: os.Args[0],
			Args:    []string{"-test.run=TestMCPProcessEnvChildProbe"},
			Env:     map[string]string{"REASONIX_ENV_PROBE_OUT": out},
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	deadline := time.Now().Add(20 * time.Second)
	for {
		if raw, readErr := os.ReadFile(out); readErr == nil {
			env := string(raw)
			if !strings.Contains(env, "REASONIX_SERVE_URL=http://127.0.0.1:8787") {
				t.Fatalf("child environment is missing the serve URL:\n%s", env)
			}
			if !strings.Contains(env, "REASONIX_RESOLVED_SESSION=") {
				t.Fatalf("the child environment does not record the resolved session:\n%s", env)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stdio MCP child never ran, so the env handoff is unproven")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(resolved) == 0 {
		t.Fatal("MCPProcessEnv was never consulted for a stdio spawn")
	}
	for _, path := range resolved {
		if strings.TrimSpace(path) == "" {
			// No session is pinned during Build; naming the root instead would be
			// the misaddressing this test exists to prevent.
			continue
		}
		if sameDirectory(t, path, workspace) {
			t.Fatalf("MCPProcessEnv session = %q, want the controller's session, not the workspace root", path)
		}
	}
}

// TestMCPProcessEnvChildProbe is the stdio child of the test above: it dumps its
// own environment and exits, so the parent can assert what the child inherited.
func TestMCPProcessEnvChildProbe(t *testing.T) {
	out := os.Getenv("REASONIX_ENV_PROBE_OUT")
	if out == "" {
		t.Skip("helper process for TestBuildInstallsMCPProcessEnvForItsRoot")
	}
	var b strings.Builder
	for _, kv := range os.Environ() {
		b.WriteString(kv)
		b.WriteString("\n")
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write env probe: %v", err)
	}
	// A helper process, not a test run: exit before the harness reports it.
	os.Exit(0)
}

// sameDirectory compares directories by identity so a resolved symlink (macOS
// /var vs /private/var, Windows 8.3 names) does not fail the assertion.
func sameDirectory(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return a == b
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) || os.SameFile(infoA, infoB)
	}
	return os.SameFile(infoA, infoB)
}
