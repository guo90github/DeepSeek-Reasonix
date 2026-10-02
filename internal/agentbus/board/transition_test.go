package board

import (
	"context"
	"strings"
	"testing"
	"time"
)

func openTestBoard(t *testing.T) *Board {
	t.Helper()
	b, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return b
}

func ctxOf(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func mustApply(t *testing.T, b *Board, op Op) Receipt {
	t.Helper()
	rec, err := b.Apply(ctxOf(t), op)
	if err != nil {
		t.Fatalf("Apply(%s %s): %v", op.Verb, op.Node, err)
	}
	return rec
}

func rejectReason(t *testing.T, b *Board, op Op) (string, bool) {
	t.Helper()
	_, err := b.Apply(ctxOf(t), op)
	if err == nil {
		return "", false
	}
	return IsReject(err)
}

func snapshot(t *testing.T, b *Board) *State {
	t.Helper()
	st, err := b.Snapshot(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return st
}

func wantState(t *testing.T, st *State, node string, want NodeState) {
	t.Helper()
	n := st.Nodes[node]
	if n == nil {
		t.Fatalf("node %q absent", node)
	}
	if n.State != want {
		t.Fatalf("node %q state = %s, want %s", node, n.State, want)
	}
}

func evidence(ref string) []Evidence { return []Evidence{{Kind: "test", Ref: ref}} }

func opAssert(node, actor string) Op {
	return Op{Verb: VerbAssert, Node: node, Actor: actor, Evidence: evidence("evidence:" + node)}
}

func opClaim(node, actor string, ttl time.Duration) Op {
	return Op{
		Verb: VerbClaim, Node: node, Actor: actor,
		Deadline: time.Now().UTC().Add(ttl),
		Bounds:   &Bounds{Steps: 5, Tokens: 1000, Output: "a result"},
	}
}

func opDecideDone(node, actor, reproducedBy string) Op {
	return Op{Verb: VerbDecide, Node: node, Actor: actor, Outcome: OutcomeDone, ReproducedBy: reproducedBy}
}

func TestAssertOnUnknownNodeCreatesItOpen(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("root", "alice"))
	st := snapshot(t, b)
	wantState(t, st, "root", StateOpen)
	if st.Applied != 1 {
		t.Fatalf("applied = %d, want 1", st.Applied)
	}
}

func TestIllegalTransitionsAreRejectedWithReason(t *testing.T) {
	live := func(b *Board) error {
		if _, err := b.Apply(context.Background(), opAssert("n", "alice")); err != nil {
			return err
		}
		_, err := b.Apply(context.Background(), opClaim("n", "alice", time.Hour))
		return err
	}
	done := func(b *Board) error {
		if _, err := b.Apply(context.Background(), opAssert("n", "producer")); err != nil {
			return err
		}
		_, err := b.Apply(context.Background(), opDecideDone("n", "judge", "checker"))
		return err
	}

	cases := []struct {
		name  string
		setup func(b *Board) error
		op    Op
		want  string
	}{
		{"claim without bounds", nil, Op{Verb: VerbClaim, Node: "n", Actor: "bob", Deadline: time.Now().UTC().Add(time.Hour)}, ReasonMissingBounds},
		{"claim with past deadline", nil, Op{Verb: VerbClaim, Node: "n", Actor: "bob", Deadline: time.Now().UTC().Add(-time.Minute), Bounds: &Bounds{Steps: 1}}, ReasonDeadlineNotFuture},
		{"claim without actor", nil, Op{Verb: VerbClaim, Node: "n", Deadline: time.Now().UTC().Add(time.Hour), Bounds: &Bounds{Steps: 1}}, ReasonMissingActor},
		{"claim unknown node", nil, opClaim("ghost", "bob", time.Hour), ReasonUnknownNode},
		{"assert without evidence", nil, Op{Verb: VerbAssert, Node: "n", Actor: "alice"}, ReasonMissingEvidence},
		{"refute unknown node", nil, Op{Verb: VerbRefute, Node: "ghost", Actor: "alice", Reason: "wrong"}, ReasonUnknownNode},
		{"refute without reason", nil, Op{Verb: VerbRefute, Node: "n", Actor: "alice"}, ReasonMissingReason},
		{"unknown verb", nil, Op{Verb: Verb("teleport"), Node: "n", Actor: "alice"}, ReasonUnknownVerb},
		{"heartbeat on open node", func(b *Board) error {
			_, err := b.Apply(context.Background(), opAssert("n", "alice"))
			return err
		}, Op{Verb: VerbHeartbeat, Node: "n", Actor: "alice", Deadline: time.Now().UTC().Add(time.Hour)}, ReasonIllegalTransition},
		{"heartbeat by non-owner", live, Op{Verb: VerbHeartbeat, Node: "n", Actor: "bob", Deadline: time.Now().UTC().Add(time.Hour)}, ReasonNotOwner},
		{"release by non-owner", live, Op{Verb: VerbRelease, Node: "n", Actor: "bob"}, ReasonNotOwner},
		{"second claim while live", live, opClaim("n", "bob", time.Hour), ReasonIllegalTransition},
		{"assert on done node", done, opAssert("n", "alice"), ReasonIllegalTransition},
		{"revert on claimed node", live, Op{Verb: VerbRevert, Node: "n", Actor: "alice"}, ReasonIllegalTransition},
		{"split on unknown node", nil, Op{Verb: VerbSplit, Node: "ghost", Children: []NodeSpec{{ID: "kid"}}}, ReasonUnknownNode},
		{"require unknown node", nil, Op{Verb: VerbRequire, Node: "ghost", Dep: &NodeSpec{ID: "dep"}}, ReasonUnknownNode},
		{"require without dep", func(b *Board) error {
			_, err := b.Apply(context.Background(), opAssert("n", "alice"))
			return err
		}, Op{Verb: VerbRequire, Node: "n"}, ReasonMissingDependency},
		{"abandon without evidence", nil, Op{Verb: VerbAbandon, Node: "n", Actor: "alice", Reason: "tried nothing"}, ReasonMissingEvidence},
		{"abandon without reason", nil, Op{Verb: VerbAbandon, Node: "n", Actor: "alice", Evidence: evidence("log")}, ReasonMissingReason},
		{"capability gap without reason", nil, Op{Verb: VerbCapabilityGap, Node: "n", Actor: "alice"}, ReasonMissingReason},
		{"decide done without assert", nil, opDecideDone("n", "judge", "checker"), ReasonUnknownNode},
		{"decide done without reproducer", func(b *Board) error {
			_, err := b.Apply(context.Background(), opAssert("n", "producer"))
			return err
		}, Op{Verb: VerbDecide, Node: "n", Actor: "judge", Outcome: OutcomeDone}, ReasonMissingReproducer},
		{"decide done self reproduced", func(b *Board) error {
			_, err := b.Apply(context.Background(), opAssert("n", "producer"))
			return err
		}, opDecideDone("n", "judge", "producer"), ReasonSelfReproduced},
		{"decide abandoned without request", func(b *Board) error {
			_, err := b.Apply(context.Background(), opAssert("n", "alice"))
			return err
		}, Op{Verb: VerbDecide, Node: "n", Actor: "judge", Outcome: OutcomeAbandoned}, ReasonMissingAbandonRequest},
		{"decide unknown outcome", func(b *Board) error {
			_, err := b.Apply(context.Background(), opAssert("n", "alice"))
			return err
		}, Op{Verb: VerbDecide, Node: "n", Actor: "judge", Outcome: Outcome("maybe")}, ReasonUnknownOutcome},
		{"decide done on abandoned node", func(b *Board) error {
			bg := context.Background()
			if _, err := b.Apply(bg, opAssert("n", "alice")); err != nil {
				return err
			}
			if _, err := b.Apply(bg, Op{Verb: VerbAbandon, Node: "n", Actor: "alice", Reason: "no path", Evidence: evidence("log")}); err != nil {
				return err
			}
			_, err := b.Apply(bg, Op{Verb: VerbDecide, Node: "n", Actor: "judge", Outcome: OutcomeAbandoned})
			return err
		}, opDecideDone("n", "judge", "checker"), ReasonIllegalTransition},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := openTestBoard(t)
			if tc.setup != nil {
				if err := tc.setup(b); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			reason, isReject := rejectReason(t, b, tc.op)
			if !isReject {
				t.Fatalf("op was accepted, want rejection %q", tc.want)
			}
			if reason != tc.want {
				t.Fatalf("reason = %q, want %q", reason, tc.want)
			}
		})
	}
}

func TestRejectedOpsLeaveNoTrace(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	before := snapshot(t, b).Seq
	if _, isReject := rejectReason(t, b, opClaim("n", "bob", time.Hour)); isReject {
		t.Fatalf("claim should have been accepted")
	}
	if _, isReject := rejectReason(t, b, Op{Verb: VerbRefute, Node: "n", Actor: "bob"}); !isReject {
		t.Fatalf("refute without reason should be rejected")
	}
	after := snapshot(t, b)
	if after.Seq != before+1 {
		t.Fatalf("seq = %d, want %d: a rejected op must not be appended", after.Seq, before+1)
	}
	if after.Rejected != 0 {
		t.Fatalf("rejected = %d, want 0: refusals never reach the log", after.Rejected)
	}
}

func TestDuplicateOpIDDoesNotAppendTwice(t *testing.T) {
	b := openTestBoard(t)
	op := opAssert("n", "alice")
	op.ID = "op-fixed-1"
	first := mustApply(t, b, op)
	second := mustApply(t, b, op)
	if first.Seq != second.Seq {
		t.Fatalf("seqs differ: %d vs %d", first.Seq, second.Seq)
	}
	if !second.Duplicate {
		t.Fatalf("retried op should report duplicate")
	}
	if got := snapshot(t, b).Applied; got != 1 {
		t.Fatalf("applied = %d, want 1", got)
	}
}

func TestRefuteMovesNodeToContested(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	mustApply(t, b, Op{Verb: VerbRefute, Node: "n", Actor: "bob", Reason: "evidence contradicts the claim"})
	st := snapshot(t, b)
	wantState(t, st, "n", StateContested)
	if len(st.Nodes["n"].Refutes) != 1 {
		t.Fatalf("refutes = %d, want 1", len(st.Nodes["n"].Refutes))
	}
}

func TestSplitCreatesChildrenAndBlocksParent(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("root", "alice"))
	mustApply(t, b, Op{Verb: VerbSplit, Node: "root", Actor: "alice", Children: []NodeSpec{{ID: "a", Title: "first"}, {ID: "b"}}})
	st := snapshot(t, b)
	wantState(t, st, "root", StateBlocked)
	wantState(t, st, "a", StateOpen)
	if !st.Nodes["a"].Ready(st) {
		t.Fatalf("a child with no dependencies should be ready")
	}
	if st.Nodes["root"].Ready(st) {
		t.Fatalf("a container must not be ready while its children are open")
	}
}

func TestSplitAndRequireRejectCycles(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("a", "alice"))
	mustApply(t, b, Op{Verb: VerbRequire, Node: "a", Actor: "alice", Dep: &NodeSpec{ID: "b"}})

	reason, isReject := rejectReason(t, b, Op{Verb: VerbRequire, Node: "b", Actor: "alice", Dep: &NodeSpec{ID: "a"}})
	if !isReject || reason != ReasonCycle {
		t.Fatalf("b requires a = (%v, %q), want cycle rejection", isReject, reason)
	}
	reason, isReject = rejectReason(t, b, Op{Verb: VerbRequire, Node: "a", Actor: "alice", Dep: &NodeSpec{ID: "a"}})
	if !isReject || reason != ReasonCycle {
		t.Fatalf("self dependency = (%v, %q), want cycle rejection", isReject, reason)
	}
	reason, isReject = rejectReason(t, b, Op{Verb: VerbSplit, Node: "b", Actor: "alice", Children: []NodeSpec{{ID: "a"}}})
	if !isReject || reason != ReasonDuplicateNode {
		t.Fatalf("split into an existing node = (%v, %q), want duplicate rejection", isReject, reason)
	}
}

func TestRevertMarksDownstreamDoneStale(t *testing.T) {
	b := openTestBoard(t)
	bg := context.Background()
	if _, err := b.ApplyAll(bg,
		opAssert("up", "p1"),
		opDecideDone("up", "judge", "checker"),
		opAssert("down", "p2"),
		Op{Verb: VerbRequire, Node: "down", Actor: "p2", Dep: &NodeSpec{ID: "up"}},
	); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A dependent cannot be decided done while it is blocked, so make it done by
	// clearing the dependency edge: assert then decide on a sibling instead.
	if _, err := b.ApplyAll(bg,
		opAssert("sibling", "p3"),
		Op{Verb: VerbRequire, Node: "sibling", Actor: "p3", Dep: &NodeSpec{ID: "up"}},
	); err != nil {
		t.Fatalf("setup sibling: %v", err)
	}
	// up depends on nothing; revert it and watch the dependency graph react.
	mustApply(t, b, Op{Verb: VerbRevert, Node: "up", Actor: "auditor"})
	st := snapshot(t, b)
	wantState(t, st, "up", StateOpen)
	if st.Nodes["up"].Outcome != "" {
		t.Fatalf("reverted node kept outcome %q", st.Nodes["up"].Outcome)
	}
	for _, id := range []string{"down", "sibling"} {
		if len(st.Nodes[id].Deps) != 1 {
			t.Fatalf("node %q lost its dependency", id)
		}
	}
}

func TestRevertMarksDoneDependentStaleAndStaleReturnsToOpen(t *testing.T) {
	b := openTestBoard(t)
	bg := context.Background()
	if _, err := b.ApplyAll(bg,
		opAssert("up", "p1"),
		opDecideDone("up", "judge", "checker"),
		opAssert("down", "p2"),
		Op{Verb: VerbRequire, Node: "down", Actor: "p2", Dep: &NodeSpec{ID: "up"}},
		opDecideDone("down", "judge", "checker"),
		opAssert("unrelated", "p3"),
		opDecideDone("unrelated", "judge", "checker"),
	); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if st := snapshot(t, b); st.Nodes["down"].State != StateDone {
		t.Fatalf("setup: down should be done, got %s", st.Nodes["down"].State)
	}
	mustApply(t, b, Op{Verb: VerbRevert, Node: "up", Actor: "auditor"})
	st := snapshot(t, b)
	wantState(t, st, "up", StateOpen)
	wantState(t, st, "down", StateStale)
	wantState(t, st, "unrelated", StateDone)
	if st.Nodes["down"].Outcome != "" {
		t.Fatalf("a stale node must not keep a done outcome, got %q", st.Nodes["down"].Outcome)
	}
	mustApply(t, b, Op{Verb: VerbRevert, Node: "down", Actor: "auditor"})
	wantState(t, snapshot(t, b), "down", StateOpen)
}

func TestAbandonRequestThenDecideAbandoned(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	mustApply(t, b, Op{Verb: VerbAbandon, Node: "n", Actor: "alice", Reason: "no reachable path", Evidence: evidence("log:attempts")})
	if st := snapshot(t, b); st.Nodes["n"].State != StateOpen {
		t.Fatalf("an abandon request must not stop the node by itself, got %s", st.Nodes["n"].State)
	}
	mustApply(t, b, Op{Verb: VerbDecide, Node: "n", Actor: "judge", Outcome: OutcomeAbandoned})
	st := snapshot(t, b)
	wantState(t, st, "n", StateAbandoned)
	if st.Nodes["n"].AbandonReq == nil {
		t.Fatalf("abandon request should stay on the node as evidence")
	}
}

func TestCapabilityGapClearsTheClaim(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	mustApply(t, b, opClaim("n", "alice", time.Hour))
	mustApply(t, b, Op{Verb: VerbCapabilityGap, Node: "n", Actor: "alice", Reason: "needs the vendored protoc plugin"})
	st := snapshot(t, b)
	wantState(t, st, "n", StateCapabilityGap)
	if st.Nodes["n"].Owner != "" {
		t.Fatalf("capability gap should release the lease, owner = %q", st.Nodes["n"].Owner)
	}
	if st.Nodes["n"].ClaimExpired(time.Now().UTC().Add(time.Hour)) {
		t.Fatalf("a released claim must not read as expired")
	}
}

func TestClaimOnExpiredClaimTakesOver(t *testing.T) {
	b := openTestBoard(t)
	bg := context.Background()
	if _, err := b.ApplyAll(bg, opAssert("n", "alice"), opClaim("n", "alice", 40*time.Millisecond)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	mustApply(t, b, opClaim("n", "bob", time.Hour))
	st := snapshot(t, b)
	if st.Nodes["n"].Owner != "bob" {
		t.Fatalf("owner = %q, want bob", st.Nodes["n"].Owner)
	}
}

func TestDeriveIDIsStableAndIntentSensitive(t *testing.T) {
	base := opAssert("n", "alice")
	first := DeriveID(base)
	second := DeriveID(base)
	if first != second {
		t.Fatalf("DeriveID is not stable")
	}
	other := opAssert("n", "bob")
	if DeriveID(base) == DeriveID(other) {
		t.Fatalf("different actors must not share an id")
	}
	if !strings.HasPrefix(DeriveID(base), "op-") {
		t.Fatalf("unexpected id shape %q", DeriveID(base))
	}
}

func TestWaitingNodeIsClaimableOnceDependenciesAreDone(t *testing.T) {
	b := openTestBoard(t)
	bg := ctxOf(t)
	if _, err := b.ApplyAll(bg,
		opAssert("root", "alice"),
		Op{Verb: VerbSplit, Node: "root", Actor: "alice", Children: []NodeSpec{{ID: "kid"}}},
	); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, isReject := rejectReason(t, b, opClaim("root", "bob", time.Hour)); !isReject {
		t.Fatalf("a container with an open child must not be claimable yet")
	}
	if _, err := b.ApplyAll(bg, opAssert("kid", "p2"), opDecideDone("kid", "judge", "checker")); err != nil {
		t.Fatalf("setup kid: %v", err)
	}
	mustApply(t, b, opClaim("root", "bob", time.Hour))
	wantState(t, snapshot(t, b), "root", StateClaimed)
}

func TestCapabilityGapCanBeClaimedAgain(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("n", "alice")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mustApply(t, b, Op{Verb: VerbCapabilityGap, Node: "n", Actor: "alice", Reason: "needs the vendored protoc plugin"})
	mustApply(t, b, opClaim("n", "bob", time.Hour))
	wantState(t, snapshot(t, b), "n", StateClaimed)
}

func TestAssertOnAwaitingNodeIsAllowed(t *testing.T) {
	b := openTestBoard(t)
	bg := ctxOf(t)
	if _, err := b.ApplyAll(bg,
		opAssert("root", "alice"),
		Op{Verb: VerbSplit, Node: "root", Actor: "alice", Children: []NodeSpec{{ID: "kid"}}},
	); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mustApply(t, b, opAssert("root", "alice"))
}

func TestRequireRejectsAClosedDependency(t *testing.T) {
	b := openTestBoard(t)
	bg := ctxOf(t)
	if _, err := b.ApplyAll(bg,
		opAssert("dep", "alice"),
		Op{Verb: VerbAbandon, Node: "dep", Actor: "alice", Reason: "no reachable path", Evidence: evidence("log")},
		Op{Verb: VerbDecide, Node: "dep", Actor: "judge", Outcome: OutcomeAbandoned},
		opAssert("n", "alice"),
	); err != nil {
		t.Fatalf("setup: %v", err)
	}
	reason, isReject := rejectReason(t, b, Op{Verb: VerbRequire, Node: "n", Actor: "alice", Dep: &NodeSpec{ID: "dep"}})
	if !isReject || reason != ReasonDependencyClosed {
		t.Fatalf("require a closed dep = (%v, %q), want dependency_closed", isReject, reason)
	}
}

func TestSplitRejectsDuplicateChildrenInOneOp(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("root", "alice")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	reason, isReject := rejectReason(t, b, Op{Verb: VerbSplit, Node: "root", Actor: "alice",
		Children: []NodeSpec{{ID: "kid"}, {ID: "kid"}}})
	if !isReject || reason != ReasonDuplicateDependency {
		t.Fatalf("duplicate children = (%v, %q), want duplicate_dependency", isReject, reason)
	}
}

func TestReusedOpIDWithDifferentIntentIsRejected(t *testing.T) {
	b := openTestBoard(t)
	first := opAssert("n", "alice")
	first.ID = "op-reused"
	mustApply(t, b, first)
	other := opAssert("n", "bob")
	other.ID = "op-reused"
	reason, isReject := rejectReason(t, b, other)
	if !isReject || reason != ReasonIdempotencyConflict {
		t.Fatalf("reused id with a new intent = (%v, %q), want idempotency_conflict", isReject, reason)
	}
	if got := snapshot(t, b).Applied; got != 1 {
		t.Fatalf("applied = %d, want 1: the collision must not be appended", got)
	}
}

func TestDeriveIDIgnoresTime(t *testing.T) {
	base := opAssert("n", "alice")
	later := opAssert("n", "alice")
	later.At = time.Now().UTC().Add(6 * time.Hour)
	if DeriveID(base) != DeriveID(later) {
		t.Fatalf("a retry hours later is the same intent and must share an id")
	}
}
