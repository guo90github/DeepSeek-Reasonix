package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func grantOp(node, actor, reason string, evidence []board.Evidence) board.Op {
	return board.Op{
		Verb: board.VerbAssert, Node: node, Actor: actor,
		Reason: reason, Source: GrantSource, Evidence: evidence,
	}
}

// grantBoard writes one capability-gap node plus whatever assertions the case needs.
func grantBoard(t *testing.T, ops ...board.Op) (string, *board.Board) {
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

func TestGrantsComeFromTheOpLogNotTheFold(t *testing.T) {
	dir, brd := grantBoard(t,
		board.Op{Verb: board.VerbAssert, Node: "need-migration", Actor: "orchestrator", Evidence: e2eEvidence()},
		grantOp("need-migration", "operator", "install the driver", e2eEvidence()),
	)
	ctx := context.Background()
	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	grants := GrantsFor(ops, "need-migration")
	if len(grants) != 1 || grants[0].Actor != "operator" {
		t.Fatalf("grants = %+v, want the one the operator gave", grants)
	}

	// The fold cannot answer this: an Assertion carries no Source, so "which assertion
	// was an authorization" is provenance and lives in the ops (§13.8).
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := len(state.Nodes["need-migration"].Asserts); got != 2 {
		t.Fatalf("asserts = %d, want both assertions folded", got)
	}
	if again := GrantsFor(ops, "need-migration"); len(again) != 1 || again[0].Seq != grants[0].Seq {
		t.Fatalf("re-reading the same ops changed the answer: %+v vs %+v", grants, again)
	}
	_ = dir
}

func TestOnlySomebodyElseWithEvidenceCanAuthorize(t *testing.T) {
	ctx := context.Background()
	// The worker holds the node: "who produced it" is what a self-grant would have to
	// beat, and that is the node's owner.
	claim := board.Op{Verb: board.VerbClaim, Node: "n", Actor: "worker",
		Bounds: &board.Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Minute)}
	worked := board.Op{Verb: board.VerbAssert, Node: "n", Actor: "worker", Evidence: e2eEvidence()}
	cases := []struct {
		name string
		ops  []board.Op
		want bool
	}{
		{
			"nobody authorized it",
			[]board.Op{worked, claim},
			false,
		},
		{
			"a worker cannot authorize itself",
			[]board.Op{worked, claim, grantOp("n", "worker", "self", e2eEvidence())},
			false,
		},
		{
			"somebody else with evidence authorizes it",
			[]board.Op{worked, claim, grantOp("n", "operator", "install the driver", e2eEvidence())},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, brd := grantBoard(t, tc.ops...)
			ops, err := brd.Ops(ctx)
			if err != nil {
				t.Fatalf("ops: %v", err)
			}
			state, err := brd.Snapshot(ctx, time.Now().UTC())
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if got := Authorized(ops, state, "n"); got != tc.want {
				t.Fatalf("authorized = %v, want %v", got, tc.want)
			}
			if grants := AuthorizedGrants(ops, state, "n"); (len(grants) > 0) != tc.want {
				t.Fatalf("authorized grants = %+v, want it to agree with Authorized", grants)
			}
		})
	}
}

func TestAnAuthorizationGoesWithTheNodeThatLostItsStanding(t *testing.T) {
	ctx := context.Background()
	evidence := e2eEvidence()
	dir, brd := grantBoard(t,
		board.Op{Verb: board.VerbAssert, Node: "n", Actor: "worker", Evidence: evidence},
		grantOp("n", "operator", "install the driver", evidence),
	)
	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !Authorized(ops, state, "n") {
		t.Fatal("the operator's grant should authorize this node")
	}

	// Taking the node's standing back must take its authorization with it.
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbRefute, Node: "n", Actor: "skeptic", Reason: "the driver was not installed", Evidence: evidence,
	}); err != nil {
		t.Fatalf("refute: %v", err)
	}
	refuted, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := refuted.Nodes["n"].State; got != board.StateContested {
		t.Fatalf("refuted node = %q, want contested: the fold must change", got)
	}
	if !Authorized(ops, refuted, "n") {
		t.Fatal("a contested node has not lost its grant yet — only a reversal does that")
	}

	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbAbandon, Node: "n", Actor: "skeptic", Reason: "cannot be installed here", Evidence: evidence,
	}); err != nil {
		t.Fatalf("abandon request: %v", err)
	}
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbDecide, Node: "n", Actor: "skeptic", Outcome: board.OutcomeAbandoned,
		Reason: "cannot be installed here", Evidence: evidence,
	}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	abandoned, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := abandoned.Nodes["n"].State; got != board.StateAbandoned {
		t.Fatalf("node = %q, want abandoned", got)
	}
	if Authorized(ops, abandoned, "n") {
		t.Fatal("an abandoned node must not still be acting on a grant")
	}
	// The record itself is never erased: who approved what stays readable.
	if len(GrantsFor(ops, "n")) != 1 {
		t.Fatalf("the grant record vanished: %+v", GrantsFor(ops, "n"))
	}

	// Replaying the same ops reaches the same answer.
	replayed, err := board.Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	replayedOps, err := replayed.Ops(ctx)
	if err != nil {
		t.Fatalf("replay ops: %v", err)
	}
	replayedState, err := replayed.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("replay snapshot: %v", err)
	}
	if Authorized(replayedOps, replayedState, "n") != Authorized(ops, abandoned, "n") {
		t.Fatal("replaying the log changed whether the node is authorized")
	}
}

// TestAnUnevidencedGrantCannotBeWritten: the "an authorization must be checkable" rule
// is belt-and-braces, because the board already refuses an assertion with no evidence.
// The guard in Authorized covers a log that arrived from somewhere else.
func TestAnUnevidencedGrantCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbAssert, Node: "n", Actor: "worker", Evidence: e2eEvidence(),
	}); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := brd.Apply(ctx, grantOp("n", "operator", "trust me", nil)); err == nil {
		t.Fatal("the board must refuse an authorization nobody could check")
	}
	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	if grants := GrantsFor(ops, "n"); len(grants) != 0 {
		t.Fatalf("grants = %+v, want none: the refused one must not be in the log", grants)
	}
}
