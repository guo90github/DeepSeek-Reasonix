package control

import (
	"context"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

func busRequire(node, actor, dep string) board.Op {
	return board.Op{Verb: board.VerbRequire, Node: node, Actor: actor, Dep: &board.NodeSpec{ID: dep}}
}

func TestWakeAgentBusWakesEachWorkSetOnceAndNeverItself(t *testing.T) {
	ctx := context.Background()
	ctrl := newAgentBusTalkController(t, t.TempDir(), "orchestrator")
	var woken []string
	ctrl.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target.Participant)
		return nil
	})

	if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if len(woken) != 0 {
		t.Fatalf("an assertion owes nobody a wake, got %v", woken)
	}

	// The writer wakes the requester as soon as the step can start: no tick needed.
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", "alice", "schema")); err != nil {
		t.Fatalf("require: %v", err)
	}
	if len(woken) != 1 || woken[0] != "alice" {
		t.Fatalf("woken = %v, want alice", woken)
	}

	// A host ticking the same board again wakes nobody a second time.
	if n := ctrl.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("a second sweep woke %d, want 0", n)
	}

	// New work wakes again.
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", "alice", "errors")); err != nil {
		t.Fatalf("second require: %v", err)
	}
	if len(woken) != 2 {
		t.Fatalf("woken = %v, want a second wake for the new step", woken)
	}

	// This session never wakes itself: waking it would queue work behind the very
	// turn that produced it.
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", "orchestrator", "budget")); err != nil {
		t.Fatalf("self require: %v", err)
	}
	if len(woken) != 2 {
		t.Fatalf("the controller woke itself: %v", woken)
	}
}

func TestWakeAgentBusWithoutAWakerWakesNobody(t *testing.T) {
	ctx := context.Background()
	ctrl := newAgentBusTalkController(t, t.TempDir(), "orchestrator")
	if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", "alice", "schema")); err != nil {
		t.Fatalf("require: %v", err)
	}
	if n := ctrl.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("without a host waker the kernel wakes nobody, got %d", n)
	}
}

func TestWakeAgentBusCountsAFailedWakeForRetry(t *testing.T) {
	ctx := context.Background()
	ctrl := newAgentBusTalkController(t, t.TempDir(), "orchestrator")
	attempts := 0
	ctrl.SetAgentBusWaker(func(_ context.Context, _ agentbus.WakeTarget) error {
		attempts++
		if attempts == 1 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", "alice", "schema")); err != nil {
		t.Fatalf("require: %v", err)
	}
	// The write path tried once and failed, so the key was released for the sweep.
	if n := ctrl.WakeAgentBus(ctx); n != 1 {
		t.Fatalf("retried wake = %d, want 1 (a failed wake must not be remembered)", n)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want the failure and the retry", attempts)
	}
	if n := ctrl.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("a delivered wake must not repeat, got %d", n)
	}
}

// A host shares one ledger across its controllers, so rebuilding one — a join, a tab
// switch, a settings change — does not re-wake work this host already delivered. The
// per-controller map could not: a join re-woke the same work set on a real machine.
func TestWakeAgentBusDedupsAcrossControllers(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ledger := NewWakeLedger()

	writer := newAgentBusTalkController(t, dir, "orchestrator")
	writer.SetAgentBusWakeLedger(ledger)
	if _, err := writer.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	// alice asked for a step that can start now, and somebody waits on it.
	if _, err := writer.ApplyAgentBusOp(ctx, busRequire("design", "alice", "schema")); err != nil {
		t.Fatalf("require: %v", err)
	}

	var woken []string
	waker := func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target.Participant)
		return nil
	}
	first := newAgentBusTalkController(t, dir, "host")
	first.SetAgentBusWakeLedger(ledger)
	first.SetAgentBusWaker(waker)
	if n := first.WakeAgentBus(ctx); n != 1 || len(woken) != 1 || woken[0] != "alice" {
		t.Fatalf("first sweep woken = %v (%d), want alice exactly once", woken, n)
	}

	second := newAgentBusTalkController(t, dir, "host")
	second.SetAgentBusWakeLedger(ledger)
	second.SetAgentBusWaker(waker)
	if n := second.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("a rebuilt controller re-woke %d for the same work set: %v", n, woken)
	}
}
