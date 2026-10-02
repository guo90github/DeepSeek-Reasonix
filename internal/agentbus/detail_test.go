package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func detailFixture(t *testing.T, ops ...board.Op) (string, *board.Board) {
	t.Helper()
	dir := t.TempDir()
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := brd.ApplyAll(context.Background(), ops...); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return dir, brd
}

func detailOf(t *testing.T, brd *board.Board, hearings *HearingState, node string) NodeDetail {
	t.Helper()
	ctx := context.Background()
	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	detail, ok := DescribeNode(state, hearings, ops, node)
	if !ok {
		t.Fatalf("node %q is not on the board", node)
	}
	return detail
}

// The detail is what a human checks a step with: what it is, what it waits for, what was
// disputed, and who authorized it — the authorization being the part only the log knows.
func TestNodeDetailReadsStateDepsDisputesAndAuthorizations(t *testing.T) {
	_, brd := detailFixture(t,
		assertOp("publish", "planner"),
		requireOp("publish", "signing-key"),
		board.Op{Verb: board.VerbRefute, Node: "publish", Actor: "skeptic", Reason: "the test does not cover the migration"},
		grantOp("publish", "operator", "the key is installed and scoped", e2eEvidence()),
	)

	detail := detailOf(t, brd, nil, "publish")
	if detail.Node != "publish" || detail.State != board.StateContested {
		t.Fatalf("detail = %+v, want the contested step", detail)
	}
	if detail.Ready {
		t.Fatal("a contested step waiting on a dependency is not ready")
	}
	if len(detail.Deps) != 1 || detail.Deps[0] != "signing-key" {
		t.Fatalf("deps = %v, want the step it waits for", detail.Deps)
	}
	if len(detail.Refutations) != 1 || detail.Refutations[0].Actor != "skeptic" {
		t.Fatalf("refutations = %+v, want the one challenge with its reason", detail.Refutations)
	}
	if detail.Refutations[0].Reason == "" {
		t.Fatal("a refutation must carry the reason it was made")
	}
	if len(detail.Authorizations) != 1 || detail.Authorizations[0].Actor != "operator" {
		t.Fatalf("authorizations = %+v, want who approved it", detail.Authorizations)
	}
}

// A grant nobody else could check, or one the producer gave itself, is not an
// authorization — the detail must not present it as one.
func TestNodeDetailLeavesOutAnAuthorizationThatDoesNotHold(t *testing.T) {
	_, brd := detailFixture(t,
		board.Op{Verb: board.VerbAssert, Node: "publish", Actor: "worker", Evidence: e2eEvidence()},
		board.Op{Verb: board.VerbClaim, Node: "publish", Actor: "worker",
			Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Hour)},
	)
	workerGrant := detailFixtureFor(t, brd, grantOp("publish", "worker", "I am happy with it", e2eEvidence()))
	detail := detailOf(t, workerGrant, nil, "publish")
	if len(detail.Authorizations) != 0 {
		t.Fatalf("authorizations = %+v, want none: nobody authorizes themselves", detail.Authorizations)
	}
}

// A question still open is part of what a reader needs to see about a step.
func TestNodeDetailCarriesTheDeliberation(t *testing.T) {
	_, brd := detailFixture(t, assertOp("publish", "planner"))
	hearings := NewHearingState()
	hearings.Hearings["publish"] = &Hearing{Node: "publish", Open: true}

	detail := detailOf(t, brd, hearings, "publish")
	if !detail.Deliberating || detail.Verdict != "" {
		t.Fatalf("deliberation = %v/%q, want an open question with no verdict yet", detail.Deliberating, detail.Verdict)
	}

	hearings.Hearings["publish"] = &Hearing{Node: "publish", Verdict: VerdictUndecided}
	settled := detailOf(t, brd, hearings, "publish")
	if settled.Deliberating || settled.Verdict != VerdictUndecided {
		t.Fatalf("deliberation = %v/%q, want it closed by rule", settled.Deliberating, settled.Verdict)
	}
}

// A node the board does not hold is reported as absent rather than rendered empty.
func TestNodeDetailReportsAnUnknownNode(t *testing.T) {
	_, brd := detailFixture(t, assertOp("publish", "planner"))
	ctx := context.Background()
	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, ok := DescribeNode(state, nil, ops, "never-existed"); ok {
		t.Fatal("an unknown node must be reported absent")
	}
	if _, ok := DescribeNode(nil, nil, ops, "publish"); ok {
		t.Fatal("no board means no detail")
	}
}

func detailFixtureFor(t *testing.T, brd *board.Board, ops ...board.Op) *board.Board {
	t.Helper()
	if _, err := brd.ApplyAll(context.Background(), ops...); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return brd
}
