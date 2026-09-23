package agentd

import (
	"context"
	"flag"
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

// TestHelperServe is not a test in the parent process: it is the child the
// manager tests start, and it answers /status exactly as `reasonix serve` does —
// same token, same path. --retire-marker additionally makes it answer /cancel
// and /shutdown the way a managed serve does and then exit; --ignore-shutdown
// answers those and stays up, which is what the kill fallback is for.
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
	allow := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if !allow(w, r) {
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	marker, ignoreShutdown := helperRetireMode(os.Args)
	if marker != "" || ignoreShutdown {
		mux.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
			if !allow(w, r) {
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
			if !allow(w, r) {
				return
			}
			w.WriteHeader(http.StatusAccepted)
			if ignoreShutdown {
				return
			}
			_ = os.WriteFile(marker, []byte("retired\n"), 0o600)
			// A real serve exits once RunGracefulListener returns, so the
			// manager's wait — not a kill — is what observes it going.
			go func() {
				time.Sleep(50 * time.Millisecond)
				os.Exit(0)
			}()
		})
	}
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

// helperRetireMode reads the retire flags, which helperArgs has no room for.
func helperRetireMode(args []string) (marker string, ignoreShutdown bool) {
	for i := range args {
		switch args[i] {
		case "--retire-marker":
			if i+1 < len(args) {
				marker = args[i+1]
			}
		case "--ignore-shutdown":
			ignoreShutdown = true
		}
	}
	return marker, ignoreShutdown
}

// helperSpec describes a launch of this test binary as a stand-in serve. The
// plain form answers only /status, so it exercises the kill fallback.
func helperSpec(t *testing.T, home, name, addr string) Spec {
	return helperSpecWithArgs(t, home, name, addr, nil)
}

// helperSpecRetiring launches a stand-in that retires itself when asked, leaving
// marker behind as proof it was asked rather than killed.
func helperSpecRetiring(t *testing.T, home, name, addr, marker string) Spec {
	return helperSpecWithArgs(t, home, name, addr, []string{"--retire-marker", marker})
}

// helperSpecShutdownIgnored launches a stand-in that accepts the shutdown
// request and stays up anyway.
func helperSpecShutdownIgnored(t *testing.T, home, name, addr string) Spec {
	return helperSpecWithArgs(t, home, name, addr, []string{"--ignore-shutdown"})
}

func helperSpecWithArgs(t *testing.T, home, name, addr string, extra []string) Spec {
	t.Helper()
	tokenFile := TokenPath(home, name)
	portFile := filepath.Join(filepath.Dir(tokenFile), name+".addr")
	argv := []string{os.Args[0],
		"-test.run=TestHelperServe", "--",
		"--addr", addr,
		"--token-file", tokenFile,
		"--port-file", portFile,
	}
	return Spec{
		Name:      name,
		Root:      home,
		Addr:      addr,
		TokenFile: tokenFile,
		PortFile:  portFile,
		LogPath:   LogPath(home, name),
		Argv:      append(argv, extra...),
	}
}

// serveBin names a real `reasonix` binary for the acceptance run:
// `go test ./internal/agentd/ -run <checks> -args --serve-bin <path>`. Without
// it the manager tests drive the stand-in instead.
var serveBin = flag.String("serve-bin", "", "path to a real reasonix binary the manager tests drive")

// realServeBin is the binary an acceptance run drives, or empty for the
// stand-in.
func realServeBin() string {
	if path := strings.TrimSpace(*serveBin); path != "" {
		return path
	}
	return strings.TrimSpace(os.Getenv("REASONIX_AGENTD_SERVE_BIN"))
}

// serveSpec is the serve a manager test launches: this test binary by default,
// or the real `reasonix serve` when an acceptance run names one. The acceptance
// run replays the same checks against the real binary, which is the only place
// the two can be compared — the stand-in once answered a bearer token that the
// real serve refused.
func serveSpec(t *testing.T, home, name, addr string) Spec {
	t.Helper()
	bin := realServeBin()
	if bin == "" {
		return helperSpec(t, home, name, addr)
	}
	// A real serve resolves its home from the process environment, so give the
	// child an isolated one instead of the machine's.
	t.Setenv("REASONIX_HOME", filepath.Join(home, "serve-home"))
	spec := helperSpec(t, home, name, addr)
	spec.Argv = []string{bin, "serve",
		"--addr", addr,
		"--auth", "token",
		"--token-file", spec.TokenFile,
		"--port-file", spec.PortFile,
		"--no-open",
	}
	return spec
}

// crashingArgv is a child that exits without ever serving, so a restart can
// never become healthy.
func crashingArgv() []string {
	if bin := realServeBin(); bin != "" {
		return []string{bin, "serve", "--definitely-not-a-flag"}
	}
	return []string{os.Args[0], "-test.run=TestHelperExitFast", "--", "--crash"}
}

func TestUpProbesAndDownStops(t *testing.T) {
	home := t.TempDir()
	addr, err := AllocateAddrIn(18900, 18960)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	manager := NewManager(home)
	rec, err := manager.Up(context.Background(), serveSpec(t, home, "ws-up", addr))
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
	spec := serveSpec(t, home, "ws-crash", addr)
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

// A deliberate stop asks the instance to end itself and then waits for that
// exit: a serve killed mid-turn loses the turn, so Down must not simply kill.
func TestDownRetiresTheInstanceRatherThanKillingIt(t *testing.T) {
	home := t.TempDir()
	addr, err := AllocateAddrIn(19045, 19059)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	marker := filepath.Join(home, "retired")
	manager := NewManager(home)
	if _, err := manager.Up(context.Background(), helperSpecRetiring(t, home, "ws-retire", addr, marker)); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := manager.Down("ws-retire"); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the instance was never asked to shut down: %v", err)
	}
	waitForNoListener(t, addr)
}

// An instance that answers the request and stays up must still be stopped: the
// fallback kills it once the shutdown window closes.
func TestDownKillsAnInstanceThatIgnoresShutdown(t *testing.T) {
	home := t.TempDir()
	addr, err := AllocateAddrIn(19090, 19110)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	manager := NewManager(home)
	manager.shutdownWait = 300 * time.Millisecond
	if _, err := manager.Up(context.Background(), helperSpecShutdownIgnored(t, home, "ws-stay", addr)); err != nil {
		t.Fatalf("Up: %v", err)
	}
	started := time.Now()
	if err := manager.Down("ws-stay"); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if elapsed := time.Since(started); elapsed < manager.shutdownWait {
		t.Fatalf("Down returned in %v, before the shutdown window closed", elapsed)
	}
	waitForNoListener(t, addr)
}

// A name stays managed while it retires: a coordinator that races the stop is told
// what is happening, and a second stop silently joins the one already in flight.
func TestUpAndDownDuringARetirement(t *testing.T) {
	home := t.TempDir()
	addr, err := AllocateAddrIn(19195, 19215)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	manager := NewManager(home)
	manager.shutdownWait = 2 * time.Second
	// This stand-in answers the shutdown request and stays up, which holds the
	// window open long enough to observe it.
	if _, err := manager.Up(context.Background(), helperSpecShutdownIgnored(t, home, "ws-race", addr)); err != nil {
		t.Fatalf("Up: %v", err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- manager.Down("ws-race") }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, upErr := manager.Up(context.Background(), helperSpec(t, home, "ws-race", addr))
		if upErr != nil && strings.Contains(upErr.Error(), "being retired") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Up during a retirement = %v, want a refusal naming the retirement", upErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := manager.Down("ws-race"); err != nil {
		t.Fatalf("a second Down must join the retirement in flight, got %v", err)
	}
	if err := <-stopped; err != nil {
		t.Fatalf("Down: %v", err)
	}
	waitForNoListener(t, addr)
}

// waitForNoListener waits until nothing accepts on addr, which is how these
// tests see a child that is really gone.
func waitForNoListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("something still listens on %s", addr)
}
