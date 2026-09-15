package plugin

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/sandbox"
	"reasonix/internal/secrets"
)

func TestStdioShellPATHProbeFiltersEnvWhenEnabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probe")
	}
	secrets.SetFilterSubprocessEnv(true)
	t.Cleanup(func() { secrets.SetFilterSubprocessEnv(false) })
	t.Setenv("REASONIX_TEST_SECRET_TOKEN", "ghp_abcdefghijklmnopqrstuvwxyz")

	out := runShellPATHCommand(context.Background(), "/bin/sh", []string{"-c", `printf 'tok=%s' "${REASONIX_TEST_SECRET_TOKEN:-none}"`})
	if !strings.Contains(string(out), "tok=none") {
		t.Fatalf("stdio shell PATH probe leaked filtered env: %q", out)
	}
}

func TestPrepareMCPPrivateStateWindowsPreservesHostTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mcp-state", "0123456789abcdef", "matlab")
	hostTemp := `C:\Users\user\AppData\Local\Temp`
	env := []string{"TMP=" + hostTemp, "TEMP=" + hostTemp, "TMPDIR=" + hostTemp}

	_, got, err := prepareMCPPrivateStateForOS(Spec{StateDir: root}, sandbox.Spec{}, env, "windows")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TMP", "TEMP", "TMPDIR"} {
		if value, ok := envValue(got, key); !ok || value != hostTemp {
			t.Fatalf("%s = %q, %v; want inherited host temp %q", key, value, ok, hostTemp)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "tmp")); !os.IsNotExist(err) {
		t.Fatalf("private Windows temp directory exists or stat failed: %v", err)
	}
	for key, want := range map[string]string{
		"XDG_CACHE_HOME": filepath.Join(root, "cache"),
		"XDG_STATE_HOME": filepath.Join(root, "state"),
	} {
		if value, ok := envValue(got, key); !ok || value != want {
			t.Fatalf("%s = %q, %v; want %q", key, value, ok, want)
		}
	}
}

func TestPrepareMCPPrivateStateUnixIsolatesTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mcp-state", "matlab")
	hostTemp := "/tmp/host"
	env := []string{"TMP=" + hostTemp, "TEMP=" + hostTemp, "TMPDIR=" + hostTemp}

	_, got, err := prepareMCPPrivateStateForOS(Spec{StateDir: root}, sandbox.Spec{}, env, "linux")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "tmp")
	for _, key := range []string{"TMP", "TEMP", "TMPDIR"} {
		if value, ok := envValue(got, key); !ok || value != want {
			t.Fatalf("%s = %q, %v; want private temp %q", key, value, ok, want)
		}
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("private Unix temp directory = (%v, %v), want directory", info, err)
	}
}

const envHelperMarker = "REASONIX_TEST_ENV_CHILD"

// The host-owned values must reach the spawned child, not only the merge
// helper: this is the boundary session-scoped wake targeting depends on.
func TestStdioSpawnDeliversHostProcessEnv(t *testing.T) {
	if os.Getenv(envHelperMarker) == "1" {
		_, _ = fmt.Fprintf(os.Stdout, "hosted=%s", os.Getenv("REASONIX_TEST_HOSTED"))
		os.Exit(0)
	}
	ctx := withHostProcessEnv(context.Background(), map[string]string{"REASONIX_TEST_HOSTED": "from-host"})
	tr, err := newStdioTransport(ctx, Spec{
		Name:    "env-probe",
		Type:    "stdio",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestStdioSpawnDeliversHostProcessEnv", "--"},
		Env:     map[string]string{envHelperMarker: "1"},
	})
	if err != nil {
		t.Fatalf("newStdioTransport: %v", err)
	}
	t.Cleanup(tr.close)
	out, _ := io.ReadAll(tr.stdout)
	if !strings.Contains(string(out), "hosted=from-host") {
		t.Fatalf("child env = %q, want the host-owned value", out)
	}
}

// Process env < host values < the server's own Env, which stays the last word.
func TestChildEnvOrdersHostValuesBelowServerEnv(t *testing.T) {
	t.Setenv("REASONIX_TEST_LAYER", "process")
	ctx := withHostProcessEnv(context.Background(), map[string]string{
		"REASONIX_TEST_LAYER":  "host",
		"REASONIX_TEST_HOSTED": "host",
	})
	env := childEnv(ctx, Spec{Env: map[string]string{
		"REASONIX_TEST_LAYER":  "server",
		"REASONIX_TEST_HOSTED": "server",
	}})
	for key, want := range map[string]string{"REASONIX_TEST_LAYER": "server", "REASONIX_TEST_HOSTED": "server"} {
		if got, ok := envValue(env, key); !ok || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, ok, want)
		}
	}
}

// No provider: the child env is process env + the server's own Env, exactly
// what every earlier build produced.
func TestChildEnvWithoutHostValuesKeepsServerEnv(t *testing.T) {
	t.Setenv("REASONIX_TEST_HOSTLESS", "process")
	env := childEnv(context.Background(), Spec{Env: map[string]string{"REASONIX_TEST_HOSTLESS": "server"}})
	if got, ok := envValue(env, "REASONIX_TEST_HOSTLESS"); !ok || got != "server" {
		t.Fatalf("REASONIX_TEST_HOSTLESS = %q, %v; want server", got, ok)
	}
	if _, ok := envValue(env, "REASONIX_SERVE_URL"); ok {
		t.Fatal("host-owned key leaked into a spawn with no provider")
	}
}

// A Host with no provider resolves nothing; installing one turns the values on
// for every later spawn.
func TestHostProcessEnvProviderRoundTrip(t *testing.T) {
	h := NewHost()
	if got := h.processEnvValues(); got != nil {
		t.Fatalf("processEnvValues() = %v, want nil", got)
	}
	h.SetProcessEnvProvider(func() map[string]string {
		return map[string]string{"REASONIX_SERVE_URL": "http://127.0.0.1:8787"}
	})
	if got := h.processEnvValues()["REASONIX_SERVE_URL"]; got != "http://127.0.0.1:8787" {
		t.Fatalf("provider value = %q", got)
	}
}
