package agentd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agent"
)

// TestHelperExitFast is the crashing child of TestSweepOpensCircuitOnACrashLoop:
// it exits without ever serving, so a restart can never become healthy.
func TestHelperExitFast(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-1] != "--crash" {
		t.Skip("helper process only")
	}
	os.Exit(1)
}

// A session another runtime already holds must not get a second writer.
func TestUpRefusesAHeldSession(t *testing.T) {
	home := t.TempDir()
	sessionPath := filepath.Join(home, "sessions", "held.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lease, err := agent.TryAcquireSessionLease(sessionPath)
	if err != nil {
		t.Skipf("cannot take a lease in this environment: %v", err)
	}
	defer lease.Release()

	manager := NewManager(home)
	spec := helperSpec(t, home, "ws-held", "")
	spec.SessionPath = sessionPath
	if _, err := manager.Up(context.Background(), spec); err == nil {
		t.Fatal("Up started a second writer for a session another lease holds")
	}
}

// Three workspaces coexist on distinct addresses: separately addressable is the
// manager's whole point.
func TestThreeInstancesStayIndependent(t *testing.T) {
	home := t.TempDir()
	manager := NewManager(home)
	ctx := context.Background()
	for i, name := range []string{"ws-one", "ws-two", "ws-three"} {
		addr, err := AllocateAddrIn(19030+i*5, 19034+i*5)
		if err != nil {
			t.Skipf("no free port: %v", err)
		}
		if _, err := manager.Up(ctx, helperSpec(t, home, name, addr)); err != nil {
			t.Fatalf("Up(%s): %v", name, err)
		}
	}
	records, err := manager.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %+v", records)
	}
	seen := map[string]bool{}
	for _, rec := range records {
		if rec.State != StateRunning {
			t.Fatalf("%s is %s, want running", rec.Name, rec.State)
		}
		if seen[rec.Addr] {
			t.Fatalf("two instances share %s", rec.Addr)
		}
		seen[rec.Addr] = true
	}
	manager.StopAll()
}

// A crash loop opens the circuit instead of restarting forever, and the state
// has to survive the next sweep: "stopped" would invite another restart.
func TestSweepOpensCircuitOnACrashLoop(t *testing.T) {
	home := t.TempDir()
	manager := NewManager(home)
	manager.launchWait = 200 * time.Millisecond
	manager.backoff = func(int) time.Duration { return 0 }

	name := "ws-loop"
	addr, err := AllocateAddrIn(19060, 19120)
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	rec := Record{
		Name: name, Root: home, Addr: addr, TokenFile: TokenPath(home, name),
		State: StateStopped,
		Argv:  []string{os.Args[0], "-test.run=TestHelperExitFast", "--", "--crash"},
	}
	if _, err := ensureToken(rec.TokenFile); err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if err := SaveRegistry(RegistryPath(home), Registry{}.Upsert(rec)); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	for i := 0; i <= crashLimit; i++ {
		if err := manager.Sweep(context.Background()); err != nil {
			t.Fatalf("Sweep %d: %v", i, err)
		}
	}
	loaded, err := LoadRegistry(RegistryPath(home))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	got, ok := loaded.Find(name)
	if !ok {
		t.Fatal("the crashing instance lost its record")
	}
	if got.State != StateCircuitOpen {
		t.Fatalf("state = %s, want %s after %d failed attempts", got.State, StateCircuitOpen, crashLimit)
	}
}
