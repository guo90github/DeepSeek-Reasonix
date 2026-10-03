package agentbus

import (
	"context"
	"slices"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func e2eEvidence() []board.Evidence {
	return []board.Evidence{{Kind: "verification", Ref: "go test ./internal/agentbus/..."}}
}

func e2eSnapshot(t *testing.T, dir string) *board.State {
	t.Helper()
	b, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open %s: %v", dir, err)
	}
	st, err := b.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return st
}

func hasDep(n *board.Node, dep string) bool {
	return slices.Contains(n.Deps, dep)
}

// planAndFinish lays down one finished chain: schema, then design on top of it.
func planAndFinish(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	evidence := e2eEvidence()
	b, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := b.ApplyAll(ctx,
		board.Op{Verb: board.VerbAssert, Node: "schema", Actor: "producer", Evidence: evidence},
		board.Op{Verb: board.VerbAssert, Node: "design", Actor: "producer", Evidence: evidence},
		board.Op{Verb: board.VerbRequire, Node: "design", Actor: "producer", Dep: &board.NodeSpec{ID: "schema"}},
	); err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, node := range []string{"schema", "design"} {
		if _, err := b.Apply(ctx, board.Op{
			Verb: board.VerbClaim, Node: node, Actor: "producer",
			Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Minute),
		}); err != nil {
			t.Fatalf("claim %s: %v", node, err)
		}
		if _, err := b.Apply(ctx, board.Op{
			Verb: board.VerbDecide, Node: node, Actor: "producer",
			Outcome: board.OutcomeDone, Evidence: evidence, ReproducedBy: "verifier",
		}); err != nil {
			t.Fatalf("finish %s: %v", node, err)
		}
	}
}

// TestARefutationChangesTheEnding is T9-2, written against the S1 migration table
// (§11.1) rather than against a guess:
//
//   - `refute` on any state but abandoned/stale ends in `contested`;
//   - `revert` is accepted on `done` or `stale`, and only it propagates: a downstream
//     `done` becomes `stale`. `abandon` deliberately does not — an abandoned
//     dependency does not retroactively falsify finished work that rested on it.
//
// All three boards fold their own log to their own ending, and folding again agrees.
func TestARefutationChangesTheEnding(t *testing.T) {
	ctx := context.Background()

	quiet := t.TempDir()
	questioned := t.TempDir()
	takenBack := t.TempDir()
	for _, dir := range []string{quiet, questioned, takenBack} {
		planAndFinish(t, dir)
	}

	questioningBoard, err := board.Open(questioned)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := questioningBoard.Apply(ctx, board.Op{
		Verb: board.VerbRefute, Node: "schema", Actor: "skeptic",
		Reason: "the test does not cover the migration", Evidence: e2eEvidence(),
	}); err != nil {
		t.Fatalf("refute: %v", err)
	}
	undoingBoard, err := board.Open(takenBack)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := undoingBoard.Apply(ctx, board.Op{
		Verb: board.VerbRevert, Node: "schema", Actor: "skeptic",
		Reason: "the conclusion does not hold", Evidence: e2eEvidence(),
	}); err != nil {
		t.Fatalf("revert a finished step: %v", err)
	}

	plain := e2eSnapshot(t, quiet)
	refuted := e2eSnapshot(t, questioned)
	reverted := e2eSnapshot(t, takenBack)

	if got := plain.Nodes["schema"].State; got != board.StateDone {
		t.Fatalf("undisputed schema = %q, want it simply done", got)
	}
	// A refutation puts the conclusion in question without touching anyone else.
	if got := refuted.Nodes["schema"].State; got != board.StateContested {
		t.Fatalf("refuted schema = %q, want contested", got)
	}
	if refuted.Nodes["design"].State != plain.Nodes["design"].State {
		t.Fatalf("a refutation alone must not falsify work that depended on it: %q then %q",
			plain.Nodes["design"].State, refuted.Nodes["design"].State)
	}
	// Taking the conclusion back is what reaches the work that rested on it.
	if got := reverted.Nodes["schema"].State; got != board.StateOpen {
		t.Fatalf("reverted schema = %q, want it back to open", got)
	}
	if got := reverted.Nodes["design"].State; got != board.StateStale {
		t.Fatalf("design = %q, want stale once its basis was taken back", got)
	}

	// Each ending is a property of its own log, not of a process.
	for _, dir := range []string{questioned, takenBack} {
		first := e2eSnapshot(t, dir)
		again := e2eSnapshot(t, dir)
		for _, node := range []string{"schema", "design"} {
			if first.Nodes[node].State != again.Nodes[node].State {
				t.Fatalf("%s/%s replayed to %q then %q", dir, node, first.Nodes[node].State, again.Nodes[node].State)
			}
		}
	}
}

// TestParticipantsGrowTheTreeWhileItRuns is T9-3: the shape of the work is not fixed
// when it starts. Someone discovers a missing step and adds it; someone else splits
// that step into parts. The tree grows at runtime and the growth is durable.
func TestParticipantsGrowTheTreeWhileItRuns(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	b, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := b.ApplyAll(ctx,
		board.Op{Verb: board.VerbAssert, Node: "design", Actor: "planner", Evidence: e2eEvidence()},
		board.Op{Verb: board.VerbClaim, Node: "design", Actor: "planner",
			Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Minute)},
	); err != nil {
		t.Fatalf("start: %v", err)
	}
	// A different participant finds a step the planner did not know about.
	if _, err := b.Apply(ctx, board.Op{
		Verb: board.VerbRequire, Node: "design", Actor: "browser", Dep: &board.NodeSpec{ID: "schema-audit"},
	}); err != nil {
		t.Fatalf("grow a dependency at runtime: %v", err)
	}
	// And a third one splits it, still at runtime.
	if _, err := b.Apply(ctx, board.Op{
		Verb: board.VerbSplit, Node: "schema-audit", Actor: "splitter",
		Children: []board.NodeSpec{{ID: "audit-1"}, {ID: "audit-2"}},
	}); err != nil {
		t.Fatalf("split at runtime: %v", err)
	}

	state := e2eSnapshot(t, dir)
	for _, id := range []string{"design", "schema-audit", "audit-1", "audit-2"} {
		if _, ok := state.Nodes[id]; !ok {
			t.Fatalf("node %q is missing: the tree must grow while it runs", id)
		}
	}
	if !hasDep(state.Nodes["design"], "schema-audit") {
		t.Fatalf("design deps = %v, want the step added at runtime", state.Nodes["design"].Deps)
	}

	// The growth is in the log, so a brand-new reader sees the same tree, and folding
	// it a second time agrees.
	replayed := e2eSnapshot(t, dir)
	if len(replayed.Nodes) != len(state.Nodes) {
		t.Fatalf("replay lost nodes: %d then %d", len(state.Nodes), len(replayed.Nodes))
	}
	if !replayed.Nodes["audit-2"].Ready(replayed) && replayed.Nodes["audit-2"].Owner == "" {
		// A split child with no dependency of its own is exactly what a participant may
		// pick up next; it must not be permanently unstartable.
		t.Fatalf("audit-2 = %+v, want it startable or taken", replayed.Nodes["audit-2"])
	}
}
