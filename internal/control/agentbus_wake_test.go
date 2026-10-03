package control

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

// The host tick is what turns a stalled step back into signal: once a step has spent the
// dispatcher's retry budget nothing about it changes any more, so a wake — addressed to
// whoever asked for it, and saying why — is the only thing left that can move it (G5).
func TestAStalledStepIsWokenBackToWhoeverAskedForIt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	alice := newAgentBusTalkController(t, dir, "alice")
	if _, err := alice.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := alice.ApplyAgentBusOp(ctx, busRequire("design", "alice", "schema")); err != nil {
		t.Fatalf("require: %v", err)
	}

	// Two hand-outs to bob, both lapsed: the sweeper records no progress for each, which is
	// exactly what stops the dispatcher from offering the step a third time.
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	for attempt := range agentBusDispatchTries {
		// Each hand-out is named by its own deadline: the sweeper keys a reclaim by
		// node + deadline, so two attempts sharing one would collapse into one record.
		if _, err := brd.Apply(ctx, board.Op{
			Verb: board.VerbClaim, Node: "schema", Actor: "bob",
			ID:       fmt.Sprintf("handout-%d", attempt),
			Bounds:   &board.Bounds{Steps: 1},
			Deadline: time.Now().UTC().Add(time.Duration(attempt+1) * time.Minute),
		}); err != nil {
			t.Fatalf("claim %d: %v", attempt, err)
		}
		if _, err := brd.Sweep(ctx, time.Now().UTC().Add(10*time.Minute)); err != nil {
			t.Fatalf("sweep %d: %v", attempt, err)
		}
	}

	var woken []agentbus.WakeTarget
	host := newAgentBusTalkController(t, dir, "host")
	host.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target)
		return nil
	})
	if n := host.WakeAgentBus(ctx); n != 1 {
		state, snapErr := brd.Snapshot(ctx, time.Now().UTC())
		_, input, probeErr := host.agentBusWakeSnapshot(ctx)
		t.Fatalf("woken = %d, want alice told her step stopped moving (snap err=%v probe err=%v node=%+v targets=%+v)",
			n, snapErr, probeErr, state.Nodes["schema"], agentbus.WakeTargets(input))
	}
	if len(woken) != 1 || woken[0].Participant != "alice" {
		t.Fatalf("woken = %+v, want alice alone", woken)
	}
	if len(woken[0].Stalled) != 1 || woken[0].Stalled[0] != "schema" {
		t.Fatalf("stalled = %v, want the step nobody will hand out again", woken[0].Stalled)
	}

	// Whoever is woken has to be able to tell why, and what to do about it.
	prompt := AgentBusWakePrompt(woken[0])
	for _, want := range []string{"schema", "stopped moving", "replan"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("wake prompt = %q, want it to contain %q", prompt, want)
		}
	}
	if line := AgentBusWakeLine(woken[0]); !strings.Contains(line, "stopped moving") {
		t.Fatalf("the human line = %q, want the same fact", line)
	}

	// The same state is one wake, however often the host ticks.
	if n := alice.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("a second tick woke %d, want 0", n)
	}
}
