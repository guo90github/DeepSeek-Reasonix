package agentbus

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

func blockerFor(t *testing.T, landed *Landing, node string) Blocker {
	t.Helper()
	for _, b := range landed.Blockers {
		if b.Node == node {
			return b
		}
	}
	t.Fatalf("no blocker for %q: %+v", node, landed.Blockers)
	return Blocker{}
}

func finishedDeliverable(ops ...board.Op) []board.Op {
	base := []board.Op{
		assertOp("publish", "planner"),
		claimOp("publish", "worker"),
		doneOp("publish", "worker", "verifier"),
	}
	return append(base, ops...)
}

// A deliverable is a node nothing depends on. Two of them, one unfinished, is not a
// landed task however green the rest of the board looks.
func TestLandingNeedsEveryDeliverableDone(t *testing.T) {
	half := testState(t, finishedDeliverable(assertOp("docs", "planner"))...)
	landed := AssessLanding(half, nil)
	if landed.Landed {
		t.Fatalf("a board with an unfinished deliverable is not landed: %s", landed.Reason)
	}
	if got := blockerFor(t, landed, "docs"); got.Kind != BlockerNotDone {
		t.Fatalf("docs blocker = %+v, want not_done", got)
	}
	if len(landed.Blockers) != 1 {
		t.Fatalf("blockers = %+v, want only the unfinished deliverable", landed.Blockers)
	}

	both := testState(t, finishedDeliverable(
		assertOp("docs", "planner"),
		claimOp("docs", "worker"), doneOp("docs", "worker", "verifier"),
	)...)
	if got := AssessLanding(both, nil); !got.Landed {
		t.Fatalf("both deliverables are done, so the task landed: %s", got.Reason)
	}
}

// The contract is explicit that a board full of done work has not landed while a
// conclusion is in doubt (§1). The dispute must be reported as a dispute, not as a
// generic "not done": what a human has to do about the two is different.
func TestLandingStopsWhileADisputeIsUnresolved(t *testing.T) {
	state := testState(t, finishedDeliverable(board.Op{
		Verb: board.VerbRefute, Node: "publish", Actor: "skeptic",
		Reason: "the test does not cover the migration", Evidence: ev("log"),
	})...)

	landed := AssessLanding(state, nil)
	if landed.Landed {
		t.Fatal("a refuted deliverable is not a landed task")
	}
	if got := state.Nodes["publish"].State; got != board.StateContested {
		t.Fatalf("setup: publish = %q, want contested", got)
	}
	got := blockerFor(t, landed, "publish")
	if got.Kind != BlockerContested {
		t.Fatalf("publish blocker = %+v, want contested rather than a plain not_done", got)
	}
	if !strings.Contains(got.Detail, "refutations") {
		t.Fatalf("detail = %q, want it to name the refutations", got.Detail)
	}
}

// Everything is done and the only thing left is a question nobody has settled.
func TestLandingStopsWhileAVerdictIsStillOut(t *testing.T) {
	state := testState(t, finishedDeliverable()...)

	hearings := NewHearingState()
	hearings.Hearings["publish"] = &Hearing{Node: "publish", Open: true}
	if got := blockerFor(t, AssessLanding(state, hearings), "publish"); got.Kind != BlockerDeliberating {
		t.Fatalf("open hearing blocker = %+v, want deliberating", got)
	}

	hearings.Hearings["publish"] = &Hearing{Node: "publish", Verdict: VerdictStands}
	if got := AssessLanding(state, hearings); !got.Landed {
		t.Fatalf("a settled question leaves nothing in doubt: %s", got.Reason)
	}

	hearings.Hearings["publish"] = &Hearing{Node: "publish", Verdict: VerdictEscalate, Escalated: true}
	if got := blockerFor(t, AssessLanding(state, hearings), "publish"); got.Kind != BlockerEscalated {
		t.Fatalf("escalated blocker = %+v, want escalated", got)
	}

	hearings.Hearings["publish"] = &Hearing{Node: "publish", Verdict: VerdictUndecided}
	if got := blockerFor(t, AssessLanding(state, hearings), "publish"); got.Kind != BlockerUndecided {
		t.Fatalf("undecided blocker = %+v, want undecided", got)
	}
}

// The verdict has to be actionable: it names the node that is actually holding the
// deliverable back, not only the deliverable that is waiting.
func TestLandingNamesWhatIsStillMissing(t *testing.T) {
	state := testState(t, assertOp("publish", "planner"), requireOp("publish", "obtain-signing-key"))
	landed := AssessLanding(state, nil)
	if landed.Landed {
		t.Fatal("a deliverable waiting on an unfinished step is not landed")
	}
	if got := blockerFor(t, landed, "publish"); got.Kind != BlockerNotDone {
		t.Fatalf("publish blocker = %+v, want not_done while it waits", got)
	}
	blocked := blockerFor(t, landed, "obtain-signing-key")
	if blocked.Kind != BlockerNotDone || !strings.Contains(blocked.Detail, "open") {
		t.Fatalf("dependency blocker = %+v, want not_done in state open", blocked)
	}

	abandoned := testState(t,
		assertOp("publish", "planner"),
		requireOp("publish", "obtain-signing-key"),
		assertOp("obtain-signing-key", "keeper"),
		board.Op{
			Verb: board.VerbAbandon, Node: "obtain-signing-key", Actor: "keeper",
			Reason: "cannot be installed here", Evidence: ev("log"),
		},
		board.Op{
			Verb: board.VerbDecide, Node: "obtain-signing-key", Actor: "keeper",
			Outcome: board.OutcomeAbandoned, Evidence: ev("log"),
		},
	)
	got := blockerFor(t, AssessLanding(abandoned, nil), "obtain-signing-key")
	if got.Kind != BlockerAbandoned {
		t.Fatalf("abandoned dependency blocker = %+v, want abandoned: it needs a revert or a replan", got)
	}

	// A dependency the board does not hold is folded state here: the write paths create
	// every node a dependency names, so only a foreign or damaged log reaches this.
	missing := testState(t, finishedDeliverable()...)
	missing.Nodes["publish"].Deps = append(missing.Nodes["publish"].Deps, "gone")
	if got := blockerFor(t, AssessLanding(missing, nil), "gone"); got.Kind != BlockerMissing {
		t.Fatalf("missing dependency blocker = %+v, want missing", got)
	}
}

func TestLandingOnAnEmptyBoard(t *testing.T) {
	got := AssessLanding(&board.State{}, nil)
	if got.Landed {
		t.Fatal("an empty board has claimed nothing, so nothing has landed")
	}
	if got.Reason == "" {
		t.Fatal("the verdict must say why it is not landed")
	}
}

// Folders read the same files and must agree: the order of blockers is part of the answer.
func TestLandingReadsTheSameTwice(t *testing.T) {
	state := testState(t,
		assertOp("publish", "planner"),
		assertOp("docs", "planner"),
		requireOp("publish", "signing-key"),
		board.Op{Verb: board.VerbRefute, Node: "docs", Actor: "skeptic", Reason: "no evidence", Evidence: ev("log")},
	)
	first := AssessLanding(state, nil)
	second := AssessLanding(state, nil)
	if first.Reason != second.Reason {
		t.Fatalf("reason drifted: %q then %q", first.Reason, second.Reason)
	}
	if len(first.Blockers) != len(second.Blockers) {
		t.Fatalf("blockers drifted: %+v then %+v", first.Blockers, second.Blockers)
	}
	for i := range first.Blockers {
		if first.Blockers[i] != second.Blockers[i] {
			t.Fatalf("blocker order drifted at %d: %+v then %+v", i, first.Blockers[i], second.Blockers[i])
		}
	}
}
