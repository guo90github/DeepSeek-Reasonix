package agentd

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHelperServe is not a test in the parent process: it is the child that
// TestUpProbesAndDownStops and TestSweepRestartsADeadInstance start, and it
// answers /status exactly as `reasonix serve` does — same token, same path.
func TestHelperServe(t *testing.T) {
	addr, tokenFile, portFile := helperArgs(os.Args)
	if addr == "" {
		t.Skip("helper process only")
	}
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("helper token: %v", err)
	}
	token := strings.TrimSpace(string(raw))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("helper listen: %v", err)
	}
	if portFile != "" {
		_ = os.WriteFile(portFile, []byte(ln.Addr().String()), 0o600)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	_ = (&http.Server{Handler: mux}).Serve(ln)
}

func helperArgs(args []string) (addr, tokenFile, portFile string) {
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--addr":
			addr = args[i+1]
		case "--token-file":
			tokenFile = args[i+1]
		case "--port-file":
			portFile = args[i+1]
		}
	}
	return addr, tokenFile, portFile
}

// helperSpec describes a launch of this test binary as a stand-in serve.
func helperSpec(t *testing.T, home, name, addr string) Spec {
	t.Helper()
	tokenFile := TokenPath(home, name)
	return Spec{
		Name:      name,
		Root:      home,
		Addr:      addr,
		TokenFile: tokenFile,
		PortFile:  filepath.Join(filepath.Dir(tokenFile), name+".addr"),
		LogPath:   LogPath(home, name),
		Argv: []string{os.Args[0],
			"-test.run=TestHelperServe", "--",
			"--addr", addr,
			"--token-file", tokenFile,
			"--port-file", filepath.Join(filepath.Dir(tokenFile), name+".addr"),
		},
	}
}

func TestUpProbesAndDownStops(t *testing.T) {
	home := t.TempDir()
	addr, err := AllocateAddrIn(18900, 18960)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	manager := NewManager(home)
	rec, err := manager.Up(context.Background(), helperSpec(t, home, "ws-up", addr))
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if rec.State != StateRunning || rec.PID == 0 {
		t.Fatalf("record = %+v", rec)
	}
	records, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(records) != 1 || records[0].State != StateRunning || records[0].LastHealthy.IsZero() {
		t.Fatalf("status = %+v", records)
	}
	if err := manager.Down("ws-up"); err != nil {
		t.Fatalf("Down: %v", err)
	}
	after, err := LoadRegistry(RegistryPath(home))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if _, ok := after.Find("ws-up"); ok {
		t.Fatal("Down left its record behind")
	}
}

func TestSweepRestartsADeadInstance(t *testing.T) {
	home := t.TempDir()
	addr, err := AllocateAddrIn(18961, 19020)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	manager := NewManager(home)
	spec := helperSpec(t, home, "ws-crash", addr)
	if _, err := manager.Up(context.Background(), spec); err != nil {
		t.Fatalf("Up: %v", err)
	}
	first := manager.liveProcess("ws-crash")
	if first == nil {
		t.Fatal("Up did not track the instance")
	}
	dead, err := os.FindProcess(first.Record.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := dead.Kill(); err != nil {
		t.Skipf("cannot kill the helper: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if err := manager.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	second := manager.liveProcess("ws-crash")
	if second == nil || second.Record.PID == first.Record.PID {
		t.Fatalf("Sweep did not replace the dead instance: %+v", second)
	}
	if err := manager.Down("ws-crash"); err != nil {
		t.Fatalf("Down: %v", err)
	}
}

func TestBackoffDelayGrowsAndCaps(t *testing.T) {
	if got := backoffDelay(1); got != backoffBase {
		t.Fatalf("backoffDelay(1) = %v", got)
	}
	if got := backoffDelay(3); got != 4*backoffBase {
		t.Fatalf("backoffDelay(3) = %v", got)
	}
	if got := backoffDelay(20); got != backoffMax {
		t.Fatalf("backoffDelay(20) = %v, want the cap", got)
	}
}

func TestCircuitOpensAfterCrashLimit(t *testing.T) {
	manager := NewManager(t.TempDir())
	if manager.circuitOpen("ws") {
		t.Fatal("a fresh instance must not start with an open circuit")
	}
	manager.mu.Lock()
	manager.watch["ws"] = &watchState{attempts: crashLimit, windowStart: manager.now(), openedAt: manager.now()}
	manager.mu.Unlock()
	if !manager.circuitOpen("ws") {
		t.Fatal("a parked instance must report an open circuit")
	}
	manager.mu.Lock()
	manager.watch["ws"].openedAt = manager.now().Add(-circuitOpenFor - time.Minute)
	manager.mu.Unlock()
	if manager.circuitOpen("ws") {
		t.Fatal("an expired circuit must allow restarts again")
	}
}

func TestEnsureTokenMintsAndReuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents", "ws.token")
	first, err := ensureToken(path)
	if err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if len(first) != 64 {
		t.Fatalf("token = %q, want 32 random bytes in hex", first)
	}
	second, err := ensureToken(path)
	if err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if second != first {
		t.Fatal("an existing token must not be rotated under a running child")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("token file: %v", err)
	}
	// Windows reports synthesized permission bits, so the mode is POSIX-only.
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("token file mode = %v, %v; want 0600", info, err)
		}
	}
}

func TestAllocateAddrRefusesAnExhaustedRange(t *testing.T) {
	if _, err := AllocateAddrIn(19021, 19021); err == nil {
		// The range is not necessarily free; only a bound port proves refusal.
		ln, listenErr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(19021)))
		if listenErr != nil {
			t.Skip("range unavailable")
		}
		defer func() { _ = ln.Close() }()
		if _, err := AllocateAddrIn(19021, 19021); err == nil {
			t.Fatal("an exhausted range must fail loudly")
		}
	}
}
