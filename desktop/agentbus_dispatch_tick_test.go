package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

// dispatchTickApp is one app whose only tab speaks as "alice" on a board with two startable
// steps waiting behind an asserted deliverable. withSession decides whether that session can
// actually be told about work: without a persisted session path the inbox refuses the wake.
func dispatchTickApp(t *testing.T, withSession bool) (*App, *control.Controller, *board.Board) {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	options := control.Options{SessionDir: dir, Sink: event.Discard}
	if withSession {
		options.SessionPath = filepath.Join(dir, "session.jsonl")
	}
	ctrl := control.New(options)
	t.Cleanup(ctrl.Close)
	boardDir := filepath.Join(t.TempDir(), "agentbus", "default")
	ctrl.SetAgentBus(boardDir, "alice")

	brd, err := board.Open(boardDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	// Two steps somebody is waiting on: only work that is waited on is dispatchable, and it is
	// written the way a session writes — through its own controller.
	for _, op := range []board.Op{
		{Verb: board.VerbAssert, Node: "design", Actor: "orchestrator", Evidence: []board.Evidence{{Kind: "test", Ref: "design.md"}}},
		{Verb: board.VerbRequire, Node: "design", Actor: "orchestrator", Dep: &board.NodeSpec{ID: "step"}},
		{Verb: board.VerbRequire, Node: "design", Actor: "orchestrator", Dep: &board.NodeSpec{ID: "step-2"}},
	} {
		if _, err := ctrl.ApplyAgentBusOp(context.Background(), op); err != nil {
			t.Fatalf("apply %s %s: %v", op.Verb, op.Node, err)
		}
	}
	return &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: ctrl}}, activeTabID: "t1"}, ctrl, brd
}

func claimsByAlice(t *testing.T, brd *board.Board) int {
	t.Helper()
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	count := 0
	for _, id := range []string{"step", "step-2"} {
		if state.Nodes[id] != nil && state.Nodes[id].Owner == "alice" {
			count++
		}
	}
	return count
}

// The host's own loop, end to end: one tick hands startable work to the participant this host
// speaks as, one step per participant per tick, and delivers the assignment into that session.
func TestADispatchTickHandsWorkToTheSessionOneStepPerTick(t *testing.T) {
	app, _, brd := dispatchTickApp(t, true)

	app.agentBusDispatchTick()
	if got := claimsByAlice(t, brd); got != 1 {
		t.Fatalf("claimed after one tick = %d, want exactly one step handed to the session", got)
	}
	// The wake the tick queued counts as work in flight, so the next tick holds the second step
	// back instead of stacking assignments on a session that has not started yet.
	app.agentBusDispatchTick()
	if got := claimsByAlice(t, brd); got != 1 {
		t.Fatalf("claimed after a second tick = %d, want the host to wait for the queued wake", got)
	}
}

// The claim is the delivery's receipt: a dispatch that cannot reach the session gives the step
// back rather than leaving it owned by a session that never heard about it (agentbus_dispatch.go).
func TestADispatchTickLeavesNoClaimWhenTheSessionCannotBeTold(t *testing.T) {
	app, _, brd := dispatchTickApp(t, false)

	app.agentBusDispatchTick()
	if got := claimsByAlice(t, brd); got != 0 {
		t.Fatalf("claimed = %d, want the step released again: nobody was told about it", got)
	}
}

// One step per tick still has to answer *which* step: with two startable steps of equal advice
// the rank's stable tie-break hands out the one required first (internal/agentbus/rank.go:56),
// and the one it skipped stays startable for a later tick instead of being dropped.
func TestADispatchTickHandsOutTheEarliestStepFirst(t *testing.T) {
	app, _, brd := dispatchTickApp(t, true)

	app.agentBusDispatchTick()
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	first, second := ownerOf(state, "step"), ownerOf(state, "step-2")
	if first != "alice" || second == "alice" {
		t.Fatalf("after one tick owners are step=%q step-2=%q, want the earliest step handed out alone", first, second)
	}
}

func ownerOf(state *board.State, node string) string {
	if entry := state.Nodes[node]; entry != nil {
		return entry.Owner
	}
	return ""
}

// A tab that never joined a board is not a dispatch target: the tick skips it rather than
// guessing a participant or a board for it.
func TestADispatchTickSkipsATabWithoutABoard(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	t.Cleanup(ctrl.Close)
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: ctrl}}, activeTabID: "t1"}

	app.agentBusDispatchTick()
}

// The rule is per participant, not per tick: one tab that can be told and one that cannot are
// handled independently in the same tick — the reachable one gets its step, and the one nobody
// could reach gives its claim back instead of swallowing the board.
func TestADispatchTickHandlesEachParticipantIndependently(t *testing.T) {
	app, alice, brd := dispatchTickApp(t, true)
	// No SessionPath on purpose: this participant's wake cannot be delivered, so its claim has to
	// come back — and that must not stop the other tab from being served in the same tick.
	silent := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	t.Cleanup(silent.Close)
	silent.SetAgentBus(alice.AgentBusDir(), "bob")
	app.tabs["t2"] = &WorkspaceTab{ID: "t2", Ctrl: silent}

	app.agentBusDispatchTick()
	if got := claimsByAlice(t, brd); got != 1 {
		t.Fatalf("alice claimed %d steps, want exactly one despite the other tab failing", got)
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if second := state.Nodes["step-2"]; second != nil && second.Owner != "" {
		t.Fatalf("step-2 owner = %q, want it left startable: nobody was told about it", second.Owner)
	}
	if entry := state.Nodes["step"]; entry == nil || entry.Owner != "alice" {
		t.Fatal("the earliest step belongs to the session that could be told")
	}
}
