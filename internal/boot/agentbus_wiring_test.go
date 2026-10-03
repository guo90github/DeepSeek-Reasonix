package boot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// TestBootAgentBusOptionsEnrollTheBuiltController pins the host seam: the board
// directory and participant id arriving through Options are enough for a
// pathless session to see its own view, with no session file involved.
func TestBootAgentBusOptionsEnrollTheBuiltController(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-agentbus-options", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-agentbus-options"
model = "x"
`)

	busDir := filepath.Join(dir, "board")
	sessions, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "evidence"}}
	if _, err := sessions.ApplyAll(context.Background(),
		board.Op{Verb: board.VerbAssert, Node: "mine", Actor: "peer", Evidence: evidence},
	); err != nil {
		t.Fatalf("apply ops: %v", err)
	}

	ctrl, err := Build(context.Background(), Options{
		Sink:        event.Discard,
		AgentBusDir: busDir,
		AgentBusID:  "peer",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if path := ctrl.SessionPath(); path != "" {
		t.Fatalf("this controller should be pathless, got %q", path)
	}

	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	if body := conversationText(reqs[0]); !strings.Contains(body, "node id=mine") {
		t.Fatalf("the enrolled view never reached the turn:\n%s", body)
	}
}

// buildEnrolledHost builds one host on a board of its own, from the config body given.
func buildEnrolledHost(t *testing.T, kind, configBody string) (*control.Controller, string) {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	provider.Register(kind, func(provider.Config) (provider.Provider, error) {
		return &effectRecordingProvider{}, nil
	})
	writeFile(t, dir, "reasonix.toml", configBody)
	busDir := filepath.Join(dir, "board")
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard, AgentBusDir: busDir, AgentBusID: "me"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctrl.Close)
	return ctrl, busDir
}

// A deliberation opened an hour ago whose required participant never answered. With the
// operator's round window applied the silent side is woken; without one the kernel counts
// nobody absent, so it is not — the state a board cannot show and only config decides
// (AGENT_BUS §11.5.7).
func countWokenForALapsedDeliberation(t *testing.T, ctrl *control.Controller, busDir string) []agentbus.WakeTarget {
	t.Helper()
	ctx := context.Background()
	log, err := agentbus.OpenHearingLog(busDir)
	if err != nil {
		t.Fatalf("open hearing log: %v", err)
	}
	if _, err := log.Append(ctx, agentbus.HearingRecord{
		Node: "design", Kind: agentbus.HearingOpen, Actor: "me", Required: []string{"bob"},
		At: time.Now().UTC().Add(-time.Hour),
	}, agentbus.HearingLimits{}); err != nil {
		t.Fatalf("open the deliberation: %v", err)
	}
	var woken []agentbus.WakeTarget
	ctrl.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target)
		return nil
	})
	if n := ctrl.WakeAgentBus(ctx); n != len(woken) {
		t.Fatalf("WakeAgentBus reported %d wakes but delivered %d", n, len(woken))
	}
	return woken
}

func TestBootAgentBusOptionsCarryTheDeliberationWindow(t *testing.T) {
	ctrl, busDir := buildEnrolledHost(t, "boot-agentbus-hearing-window", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[agentbus]
hearing_round_ttl_minutes = 1

[[providers]]
name = "test-model"
kind = "boot-agentbus-hearing-window"
model = "x"
`)
	woken := countWokenForALapsedDeliberation(t, ctrl, busDir)
	if len(woken) != 1 || woken[0].Participant != "bob" {
		t.Fatalf("woken = %+v, want bob woken to answer: the configured window makes his silence countable", woken)
	}
	if len(woken[0].Owes) != 1 || woken[0].Owes[0] != "design" {
		t.Fatalf("owes = %v, want the deliberation he never answered", woken[0].Owes)
	}
}

func TestBootAgentBusWithoutAWindowWakesNobody(t *testing.T) {
	ctrl, busDir := buildEnrolledHost(t, "boot-agentbus-hearing-nowindow", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-agentbus-hearing-nowindow"
model = "x"
`)
	if woken := countWokenForALapsedDeliberation(t, ctrl, busDir); len(woken) != 0 {
		t.Fatalf("woken = %+v, want nobody: with no [agentbus] window the kernel never counts silence", woken)
	}
}
