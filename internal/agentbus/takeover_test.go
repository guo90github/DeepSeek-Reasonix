package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// TestWorkAdvancesWithoutTheOrchestrator is T9-1: the participant that planned the
// work is gone — its process, its handles, its in-memory state. A different
// participant reads only the files and still recognises, claims and finishes the
// work. This is the difference between a cluster and a work dispatcher: a dispatcher
// stops when its dispatcher stops.
//
// Cross-process log semantics (three real writers, a killed writer, torn tails) are
// covered by the board package's subprocess tests; this adds the takeover dimension
// on the same real files.
func TestWorkAdvancesWithoutTheOrchestrator(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	evidence := []board.Evidence{{Kind: "verification", Ref: "go test ./internal/agentbus/..."}}

	// --- the orchestrator plans the tree, claims one step and parks the other.
	orchestrator, err := board.Open(dir)
	if err != nil {
		t.Fatalf("orchestrator open: %v", err)
	}
	if _, err := orchestrator.ApplyAll(ctx,
		board.Op{Verb: board.VerbAssert, Node: "design", Actor: "orchestrator", Evidence: evidence},
		board.Op{Verb: board.VerbRequire, Node: "design", Actor: "orchestrator", Dep: &board.NodeSpec{ID: "schema"}},
		board.Op{Verb: board.VerbRequire, Node: "design", Actor: "orchestrator", Dep: &board.NodeSpec{ID: "migration"}},
	); err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, err := orchestrator.Apply(ctx, board.Op{
		Verb: board.VerbClaim, Node: "schema", Actor: "orchestrator",
		Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(60 * time.Millisecond),
	}); err != nil {
		t.Fatalf("orchestrator claim: %v", err)
	}
	queue, err := OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	if _, _, err := queue.Enqueue(ctx, QueueEntry{
		Node: "migration", Subtree: "design", Participant: "orchestrator",
	}, QueueLimits{}); err != nil {
		t.Fatalf("park: %v", err)
	}
	// Everything below is forbidden from touching the orchestrator: no handle, no
	// state, no help. Its lease lapses while nobody is watching.
	orchestrator = nil

	time.Sleep(120 * time.Millisecond)

	// --- the survivor sees the same files and takes over.
	survivor, err := board.Open(dir)
	if err != nil {
		t.Fatalf("survivor open: %v", err)
	}
	if _, err := survivor.Sweep(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	reclaimed, err := survivor.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if owner := reclaimed.Nodes["schema"].Owner; owner != "" {
		t.Fatalf("schema owner = %q after the lease lapsed, want nobody", owner)
	}
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbClaim, Node: "schema", Actor: "survivor",
		Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Minute),
	}); err != nil {
		t.Fatalf("survivor claim: %v", err)
	}
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbAssert, Node: "schema", Actor: "survivor", Evidence: evidence,
	}); err != nil {
		t.Fatalf("survivor assert: %v", err)
	}
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbDecide, Node: "schema", Actor: "survivor",
		Outcome: board.OutcomeDone, Evidence: evidence, ReproducedBy: "verifier",
	}); err != nil {
		t.Fatalf("survivor decide: %v", err)
	}

	// The parked step is still parked, and still nobody's but the board's: a host slot
	// and the queue are enough to start it.
	ledger := NewLedger(BudgetLimits{Slots: 1, Board: 100, Subtree: 100})
	taken, err := Take(ctx, queue, ledger, nil, "survivor", 1, QueueLimits{})
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if len(taken) != 1 || taken[0].Node != "migration" {
		t.Fatalf("taken = %+v, want the parked step", taken)
	}
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbClaim, Node: "migration", Actor: "survivor",
		Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Minute),
	}); err != nil {
		t.Fatalf("claim parked step: %v", err)
	}
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbAssert, Node: "migration", Actor: "survivor", Evidence: evidence,
	}); err != nil {
		t.Fatalf("assert parked step: %v", err)
	}
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbDecide, Node: "migration", Actor: "survivor",
		Outcome: board.OutcomeDone, Evidence: evidence, ReproducedBy: "verifier",
	}); err != nil {
		t.Fatalf("finish parked step: %v", err)
	}

	// --- a third, brand-new reader: the work advanced, and nothing needs attention.
	reader, err := board.Open(dir)
	if err != nil {
		t.Fatalf("reader open: %v", err)
	}
	state, err := reader.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("reader snapshot: %v", err)
	}
	for _, node := range []string{"schema", "migration"} {
		if got := state.Nodes[node].Outcome; got != board.OutcomeDone {
			t.Fatalf("%s outcome = %q, want it finished after the orchestrator died", node, got)
		}
	}
	if !state.Nodes["design"].Ready(state) {
		t.Fatalf("design = %+v, want the container ready now that its steps are done", state.Nodes["design"])
	}

	hearingLog, err := OpenHearingLog(dir)
	if err != nil {
		t.Fatalf("open hearing log: %v", err)
	}
	hearings, _, err := hearingLog.Read()
	if err != nil {
		t.Fatalf("read hearings: %v", err)
	}
	queueState, _, err := queue.Read()
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	briefing := Observe(state, queueState, hearings, time.Now().UTC(), ObserveLimits{})
	for _, signal := range briefing.Signals {
		if signal.Kind.Mandatory() {
			t.Fatalf("a finished board still reports %s on %s: %s", signal.Kind, signal.Node, signal.Detail)
		}
	}

	// Replaying the same files must reach the same place: nothing about the takeover
	// lived in a process.
	again, err := board.Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	replayed, err := again.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("replay snapshot: %v", err)
	}
	if replayed.Nodes["schema"].Outcome != state.Nodes["schema"].Outcome ||
		replayed.Nodes["migration"].Outcome != state.Nodes["migration"].Outcome {
		t.Fatalf("replay diverged: %+v vs %+v", replayed.Nodes["schema"], state.Nodes["schema"])
	}
}
