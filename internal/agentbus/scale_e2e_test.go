package agentbus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// The scale of T9-6: 10 subtrees of 10 nodes, worked by 20 participants, and every fifth
// participant gone. One in five is the 20% the contract asks for; which one is fixed rather
// than drawn at random so a failure can be reproduced from the test alone.
const (
	scaleSubtrees        = 10
	scaleStepsPerSubtree = 9
	scaleParticipants    = 20
	scaleKilledEvery     = 5
)

func scaleSubtree(i int) string { return fmt.Sprintf("work-%02d", i) }
func scaleRoot(i int) string    { return scaleSubtree(i) }
func scaleStep(i, j int) string { return fmt.Sprintf("%s/step-%02d", scaleSubtree(i), j) }
func scaleWorker(i int) string  { return fmt.Sprintf("worker-%02d", i) }

func scaleStepOwner(i, j int) string {
	return scaleWorker((i*scaleStepsPerSubtree + j) % scaleParticipants)
}
func scaleRootOwner(i int) string { return scaleWorker((i * 3) % scaleParticipants) }

// scaleGone reports whether a participant is one of the killed: its process, its handles and
// its in-memory state are not available to anything below.
func scaleGone(worker string) bool {
	for i := 0; i < scaleParticipants; i += scaleKilledEvery {
		if scaleWorker(i) == worker {
			return true
		}
	}
	return false
}

func scaleIDs() []string {
	out := make([]string, 0, scaleSubtrees*(scaleStepsPerSubtree+1))
	for i := 0; i < scaleSubtrees; i++ {
		out = append(out, scaleRoot(i))
		for j := 0; j < scaleStepsPerSubtree; j++ {
			out = append(out, scaleStep(i, j))
		}
	}
	return out
}

// scalePlan lays the tree down: each root depends on nine steps, so the board holds 100
// nodes with real dependencies and ten deliverables. Every step is asked for by the
// participant that works on it, because "who asked for this" lives in the op log and is
// what the wake surface reads (AGENT_BUS §13.2).
func scalePlan(t *testing.T, brd *board.Board) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < scaleSubtrees; i++ {
		ops := []board.Op{{Verb: board.VerbAssert, Node: scaleRoot(i), Actor: scaleRootOwner(i), Evidence: e2eEvidence()}}
		for j := 0; j < scaleStepsPerSubtree; j++ {
			ops = append(ops, board.Op{
				Verb: board.VerbRequire, Node: scaleRoot(i), Actor: scaleStepOwner(i, j),
				Dep: &board.NodeSpec{ID: scaleStep(i, j), Title: "a step of " + scaleRoot(i)},
			})
		}
		if _, err := brd.ApplyAll(ctx, ops...); err != nil {
			t.Fatalf("plan %s: %v", scaleRoot(i), err)
		}
	}
}

// scaleAssemble completes one node the way the contract requires: an evidenced assertion,
// then a verdict somebody else can reproduce. A claim comes first when the caller is starting
// the work rather than finishing its own.
func scaleAssemble(t *testing.T, brd *board.Board, node, actor string, claim bool, lease time.Duration) {
	t.Helper()
	ctx := context.Background()
	if claim {
		if _, err := brd.Apply(ctx, board.Op{
			Verb: board.VerbClaim, Node: node, Actor: actor,
			Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(lease),
		}); err != nil {
			t.Fatalf("claim %s for %s: %v", node, actor, err)
		}
	}
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbAssert, Node: node, Actor: actor, Evidence: e2eEvidence(),
	}); err != nil {
		t.Fatalf("assert %s by %s: %v", node, actor, err)
	}
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbDecide, Node: node, Actor: actor,
		Outcome: board.OutcomeDone, Evidence: e2eEvidence(), ReproducedBy: "verifier",
	}); err != nil {
		t.Fatalf("decide %s by %s: %v", node, actor, err)
	}
}

// TestAHundredNodesConvergeAfterLosingAFifthOfTheParticipants is T9-6: the board is big, a
// fifth of the participants are gone, and the work still lands — with a bounded log, nothing
// requested once it is done, and nothing on the human screen asking for attention.
func TestAHundredNodesConvergeAfterLosingAFifthOfTheParticipants(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	dir := t.TempDir()
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	scalePlan(t, brd)
	ids := scaleIDs()
	if len(ids) != 100 {
		t.Fatalf("setup: the board plans %d nodes, want 100", len(ids))
	}

	// Everyone takes their steps: the live workers finish theirs, while the killed ones left a
	// claim behind that lapses on its own — which is all a crash leaves. A lease has to be in
	// the future to be written at all, so this one is short and the clock moves past it.
	lost := 0
	for i := 0; i < scaleSubtrees; i++ {
		for j := 0; j < scaleStepsPerSubtree; j++ {
			node, owner := scaleStep(i, j), scaleStepOwner(i, j)
			if !scaleGone(owner) {
				scaleAssemble(t, brd, node, owner, true, time.Hour)
				continue
			}
			lost++
			if _, err := brd.Apply(ctx, board.Op{
				Verb: board.VerbClaim, Node: node, Actor: owner,
				Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(30 * time.Millisecond),
			}); err != nil {
				t.Fatalf("claim %s: %v", node, err)
			}
		}
	}
	if lost == 0 {
		t.Fatal("setup: nobody was lost, so there is nothing to take over")
	}
	beforeCrash := e2eSnapshot(t, dir)
	finishedBeforeCrash := 0
	for _, id := range ids {
		if beforeCrash.Nodes[id].State == board.StateDone {
			finishedBeforeCrash++
		}
	}
	if finishedBeforeCrash == 0 {
		t.Fatal("setup: nothing was finished before the crash, so there is nothing to preserve")
	}

	// The survivors see only the files. The sweeper reclaims what the dead still held and the
	// record says so, without erasing the work the dead had already finished. It runs with the
	// clock past those short leases: reclaiming is a function of the record, not of waiting.
	sweepAt := time.Now().UTC().Add(time.Second)
	receipts, err := brd.Sweep(ctx, sweepAt)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(receipts) != lost {
		t.Fatalf("sweep reclaimed %d claims, want the %d whose owner is gone", len(receipts), lost)
	}
	afterSweep := e2eSnapshot(t, dir)
	for i := 0; i < scaleSubtrees; i++ {
		for j := 0; j < scaleStepsPerSubtree; j++ {
			node := scaleStep(i, j)
			want := 0
			if scaleGone(scaleStepOwner(i, j)) {
				want = 1
			}
			if got := afterSweep.Nodes[node].NoProgress; got != want {
				t.Fatalf("%s: noProgress = %d after the sweep, want %d (%s)",
					node, got, want, afterSweep.Nodes[node].State)
			}
		}
	}
	for _, id := range ids {
		if beforeCrash.Nodes[id].State == board.StateDone && afterSweep.Nodes[id].State != board.StateDone {
			t.Fatalf("%s was done before the crash and is %q now", id, afterSweep.Nodes[id].State)
		}
	}

	// Survivors take over the reclaimed steps and assemble each root once its steps are done.
	// Each round finishes whatever can finish now; a sixth round would mean it stopped
	// converging.
	for round := 0; ; round++ {
		state := e2eSnapshot(t, dir)
		verdict := AssessLanding(state, nil)
		if verdict.Landed {
			break
		}
		if round > 5 {
			t.Fatalf("the board stopped converging: %s", verdict.Reason)
		}
		for _, id := range ids {
			n := state.Nodes[id]
			switch {
			case n.State == board.StateClaimed && !scaleGone(n.Owner):
				scaleAssemble(t, brd, id, n.Owner, false, 0)
			case n.State == board.StateOpen:
				scaleAssemble(t, brd, id, scaleWorker(1), true, time.Hour)
			case n.State == board.StateBlocked && n.Ready(state):
				scaleAssemble(t, brd, id, scaleWorker(2), true, time.Hour)
			}
		}
	}

	// The deliverables landed, and nothing the dead held is still holding the board back.
	final := e2eSnapshot(t, dir)
	if verdict := AssessLanding(final, nil); !verdict.Landed {
		t.Fatalf("the task did not land: %s", verdict.Reason)
	}
	if len(final.Nodes) != len(ids) {
		t.Fatalf("nodes = %d, want %d", len(final.Nodes), len(ids))
	}
	for i := 0; i < scaleSubtrees; i++ {
		if got := final.Nodes[scaleRoot(i)].State; got != board.StateDone {
			t.Fatalf("deliverable %s = %q, want done", scaleRoot(i), got)
		}
	}

	// A later sweep must find nothing: a cluster that keeps reclaiming finished work is how
	// small bookkeeping turns into a storm.
	if extra, err := brd.Sweep(ctx, now.Add(time.Hour)); err != nil || len(extra) != 0 {
		t.Fatalf("a later sweep reclaimed %d claims (%v), want none", len(extra), err)
	}
	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	if targets := WakeTargets(WakeInput{Ops: ops, Now: now}); len(targets) != 0 {
		t.Fatalf("wake targets after landing = %+v, want none", targets)
	}

	// And the human screen has nothing to say: no orphan, no stall, no dispute.
	briefing := Observe(final, nil, nil, now, ObserveLimits{})
	if len(briefing.Signals) != 0 || len(briefing.Cards) != 0 {
		t.Fatalf("briefing after landing = %d cards, %d signals, want none: %+v",
			len(briefing.Cards), len(briefing.Signals), briefing.Signals)
	}

	// The log stays bounded: per node one assertion and one verdict, per step one claim and
	// one requirement, the lost cost one reclaim each, and the roots one claim. A storm of
	// retries would not fit in this budget.
	if budget := 6 * len(ids); len(ops) > budget {
		t.Fatalf("ops = %d, want at most %d: something is retrying", len(ops), budget)
	}

	// A brand-new reader folds the same ending from the same files.
	if again := e2eSnapshot(t, dir); !AssessLanding(again, nil).Landed {
		t.Fatalf("replaying the log did not reach the landing: %s", AssessLanding(again, nil).Reason)
	}
}
