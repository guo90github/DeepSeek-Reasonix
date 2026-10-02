package board

import (
	"testing"
	"time"
)

func TestSweepReclaimsExpiredClaimAndRecordsNoProgress(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("n", "alice"), opClaim("n", "alice", 40*time.Millisecond)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	time.Sleep(90 * time.Millisecond)

	now := time.Now().UTC()
	recs, err := b.Sweep(ctxOf(t), now)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("sweep receipts = %d, want 1", len(recs))
	}
	st := snapshot(t, b)
	wantState(t, st, "n", StateOpen)
	if st.Nodes["n"].Owner != "" {
		t.Fatalf("owner = %q, want cleared", st.Nodes["n"].Owner)
	}
	if st.Nodes["n"].NoProgress != 1 {
		t.Fatalf("noProgress = %d, want 1", st.Nodes["n"].NoProgress)
	}

	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	rec := read.Ops[len(read.Ops)-1]
	if rec.Verb != VerbNoProgress || rec.Actor != ActorSystem {
		t.Fatalf("reclaim record = %+v, want a system no_progress op", rec)
	}
}

func TestSweepIsIdempotent(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("n", "alice"), opClaim("n", "alice", 40*time.Millisecond)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	time.Sleep(90 * time.Millisecond)
	now := time.Now().UTC()
	if _, err := b.Sweep(ctxOf(t), now); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	second, err := b.Sweep(ctxOf(t), now)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second sweep = %d receipts, want 0", len(second))
	}
	if st := snapshot(t, b); st.Nodes["n"].NoProgress != 1 {
		t.Fatalf("noProgress = %d, want 1: reclamation must be idempotent", st.Nodes["n"].NoProgress)
	}
}

func TestSweepLeavesLiveClaimsAlone(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("n", "alice"), opClaim("n", "alice", time.Hour)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	recs, err := b.Sweep(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("sweep = %d receipts, want 0", len(recs))
	}
	st := snapshot(t, b)
	wantState(t, st, "n", StateClaimed)
	if st.Nodes["n"].Owner != "alice" {
		t.Fatalf("owner = %q, want alice", st.Nodes["n"].Owner)
	}
}

func TestHeartbeatExtendsTheLeaseAndSweepSparesIt(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("n", "alice"), opClaim("n", "alice", 60*time.Millisecond)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := b.Apply(ctxOf(t), Op{Verb: VerbHeartbeat, Node: "n", Actor: "alice", Deadline: time.Now().UTC().Add(time.Hour)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	time.Sleep(90 * time.Millisecond)
	recs, err := b.Sweep(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("a renewed lease must survive a sweep, got %d receipts", len(recs))
	}
	wantState(t, snapshot(t, b), "n", StateClaimed)
}

func TestNoProgressIsSystemOnly(t *testing.T) {
	b := openTestBoard(t)
	if _, err := b.ApplyAll(ctxOf(t), opAssert("n", "alice"), opClaim("n", "alice", 40*time.Millisecond)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	time.Sleep(90 * time.Millisecond)
	for _, actor := range []string{"alice", "bob", ActorSystem} {
		reason, isReject := rejectReason(t, b, Op{Verb: VerbNoProgress, Node: "n", Actor: actor})
		if !isReject || reason != ReasonSystemOnly {
			t.Fatalf("Apply(no_progress by %s) = (%v, %q), want system_only", actor, isReject, reason)
		}
	}
	recs, err := b.Sweep(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("sweep receipts = %d, want 1: Sweep is the sanctioned path", len(recs))
	}
}

func TestSweepReclaimsSeveralExpiredClaimsInOnePass(t *testing.T) {
	b := openTestBoard(t)
	ops := []Op{}
	for _, id := range []string{"a", "b", "c"} {
		ops = append(ops, opAssert(id, "alice"), opClaim(id, "alice", 40*time.Millisecond))
	}
	if _, err := b.ApplyAll(ctxOf(t), ops...); err != nil {
		t.Fatalf("setup: %v", err)
	}
	time.Sleep(90 * time.Millisecond)
	recs, err := b.Sweep(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("sweep receipts = %d, want 3", len(recs))
	}
	st := snapshot(t, b)
	for _, id := range []string{"a", "b", "c"} {
		wantState(t, st, id, StateOpen)
		if st.Nodes[id].NoProgress != 1 {
			t.Fatalf("node %q noProgress = %d, want 1", id, st.Nodes[id].NoProgress)
		}
	}
}
