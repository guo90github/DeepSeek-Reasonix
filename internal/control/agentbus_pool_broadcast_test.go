package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// poolWakeFor seeds one board with one pooled step and one idle session on its roster, then runs
// one sweep and reports who was woken.
func poolWakeFor(t *testing.T, seed func(t *testing.T, c *Controller)) map[string]agentbus.WakeTarget {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	host := newAgentBusTalkController(t, dir, "host")
	// A session with no work of its own, on the same board: it is the audience the pool has to
	// reach, and no other derivation on the board would ever name it (F53).
	peer := newAgentBusTalkController(t, dir, "peer")
	if err := peer.AgentBusAnnounce("", ""); err != nil {
		t.Fatalf("announce peer: %v", err)
	}
	seed(t, host)

	woken := map[string]agentbus.WakeTarget{}
	host.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken[target.Participant] = target
		return nil
	})
	host.WakeAgentBus(ctx)
	return woken
}

// The pool is work nobody holds and nobody waits on, so no derivation on the board names it and
// the idle session — the one that could take it — is exactly who never heard. This is the push
// half of F53; the pull half is action=pool (2026-10-05).
func TestTheSweepTellsTheRosterAboutStepsInThePool(t *testing.T) {
	ctx := context.Background()
	woken := poolWakeFor(t, func(t *testing.T, c *Controller) {
		if _, err := c.ApplyAgentBusOp(ctx, busAssert("loose", "alice")); err != nil {
			t.Fatalf("assert loose: %v", err)
		}
	})

	got, ok := woken["peer"]
	if !ok {
		t.Fatalf("woken = %+v, want the idle session told about the pool", woken)
	}
	if len(got.Pool) != 1 || got.Pool[0] != "loose" {
		t.Fatalf("pool = %v, want the step nobody holds", got.Pool)
	}
	if line := AgentBusWakeLine(got); !strings.Contains(line, "nobody holds") {
		t.Fatalf("wake line = %q, want the pool named", line)
	}
	if prompt := AgentBusWakePrompt(got); !strings.Contains(prompt, "loose") {
		t.Fatalf("wake prompt = %q, want the pooled step named", prompt)
	}
}

// A board write already pushes the pool — the board wakes whoever it owes on every move — so a
// sweep adds nothing for a pool it has already named, while a pool that changed is news again.
// That is the bound: one push per pool state, not one per tick (F53, 2026-10-05).
func TestAnUnchangedPoolIsNotPushedTwice(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	host := newAgentBusTalkController(t, dir, "host")
	peer := newAgentBusTalkController(t, dir, "peer")
	if err := peer.AgentBusAnnounce("", ""); err != nil {
		t.Fatalf("announce peer: %v", err)
	}

	var keys []string
	host.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		keys = append(keys, target.Key)
		return nil
	})

	if _, err := host.ApplyAgentBusOp(ctx, busAssert("loose", "alice")); err != nil {
		t.Fatalf("assert loose: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("deliveries after the first write = %v, want the pool pushed once", keys)
	}
	if n := host.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("sweep woke %d, want none: that pool was already named", n)
	}

	// A pool that changed is news again.
	if _, err := host.ApplyAgentBusOp(ctx, busAssert("second", "alice")); err != nil {
		t.Fatalf("assert second: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("deliveries = %v, want the changed pool pushed again", keys)
	}
	if keys[0] == keys[1] {
		t.Fatalf("both pushes carry key %q: a changed pool cannot be told from the old one", keys[0])
	}
	if n := host.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("sweep woke %d, want none: the newest pool was already named", n)
	}
}

// An empty pool wakes nobody: there is nothing to point at.
func TestAnEmptyPoolWakesNobody(t *testing.T) {
	woken := poolWakeFor(t, func(t *testing.T, c *Controller) {})
	if len(woken) != 0 {
		t.Fatalf("woken = %+v, want nobody told about an empty board", woken)
	}
}
