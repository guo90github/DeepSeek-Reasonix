package board

import (
	"fmt"
	"testing"
	"time"
)

// assertAs builds an assertion carrying its own write time and evidence, so a test can
// place moves inside or outside the rate window and still get distinct op ids.
func assertAs(node string, at time.Time, ref string) Op {
	return Op{Verb: VerbAssert, Node: node, Actor: "alice", Evidence: evidence(ref), At: at}
}

func openRateBoard(t *testing.T, perMinute int) *Board {
	t.Helper()
	b, err := Open(t.TempDir(), Limits{NodeRatePerMinute: perMinute})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return b
}

// AGENT_BUS §S5's fourth hard limit is a ceiling on how often one node may move ("every
// node, per minute"). It is a write-time guard measured off the log's own timestamps, so
// what the log holds is never re-judged on replay.
func TestTheNodeRateCeilingStopsTheMoveAfterTheLastOneAllowed(t *testing.T) {
	now := time.Now().UTC()
	b := openRateBoard(t, 3)
	for i := range 3 {
		mustApply(t, b, assertAs("root", now.Add(time.Duration(i)*time.Second), fmt.Sprintf("e%d", i)))
	}
	reason, refused := rejectReason(t, b, assertAs("root", now.Add(3*time.Second), "e3"))
	if !refused {
		t.Fatalf("the fourth move inside the window was accepted, want %q", ReasonRateLimited)
	}
	if reason != ReasonRateLimited {
		t.Fatalf("the fourth move was refused with %q, want %q", reason, ReasonRateLimited)
	}
}

func TestTheNodeRateWindowIsMeasuredFromTheLogsOwnTimestamps(t *testing.T) {
	now := time.Now().UTC()
	b := openRateBoard(t, 1)
	mustApply(t, b, assertAs("root", now, "e1"))
	// The first move is older than the window, so it no longer counts and this one lands.
	mustApply(t, b, assertAs("root", now.Add(2*time.Minute), "e2"))
	reason, refused := rejectReason(t, b, assertAs("root", now.Add(2*time.Minute+time.Second), "e3"))
	if !refused || reason != ReasonRateLimited {
		t.Fatalf("the move after the window's single move = %q, %v; want %q", reason, refused, ReasonRateLimited)
	}
}

func TestAHeartbeatIsNotANodeMove(t *testing.T) {
	// A long claim heartbeats to keep its lease: counting renewals as moves would throttle
	// the very lease the cluster leans on, so only moves are counted.
	now := time.Now().UTC()
	b := openRateBoard(t, 2)
	mustApply(t, b, assertAs("root", now, "e1"))
	mustApply(t, b, opClaim("root", "alice", time.Hour))
	mustApply(t, b, Op{
		Verb: VerbHeartbeat, Node: "root", Actor: "alice",
		Deadline: time.Now().UTC().Add(time.Hour), At: now.Add(time.Second),
	})
	reason, refused := rejectReason(t, b, assertAs("root", now.Add(2*time.Second), "e2"))
	if !refused || reason != ReasonRateLimited {
		t.Fatalf("the third move = %q, %v; want %q (the heartbeat must not have counted)", reason, refused, ReasonRateLimited)
	}
}

func TestNoNodeRateCeilingMeansNoRefusal(t *testing.T) {
	// Zero leaves the bound off, as every other kernel limit does: the kernel invents no
	// ceilings on the operator's behalf.
	b := openTestBoard(t)
	now := time.Now().UTC()
	for i := range 20 {
		mustApply(t, b, assertAs("root", now, fmt.Sprintf("e%d", i)))
	}
}

func TestTheNodeRateCeilingCountsEachNodeOnItsOwn(t *testing.T) {
	now := time.Now().UTC()
	b := openRateBoard(t, 1)
	mustApply(t, b, assertAs("a", now, "e1"))
	mustApply(t, b, assertAs("b", now, "e1"))
	reason, refused := rejectReason(t, b, assertAs("a", now.Add(time.Second), "e2"))
	if !refused || reason != ReasonRateLimited {
		t.Fatalf("a's second move = %q, %v; want %q (b's move must not count against a)", reason, refused, ReasonRateLimited)
	}
}

func TestARefusedMoveLeavesNoTraceAndReplayAgrees(t *testing.T) {
	// The guard keeps a refused move out of the log. What remains reads the same ways: through
	// the writer's own fold, by folding the recorded ops, and through a fresh handle — because
	// the ceiling is re-derived from the log rather than stored anywhere.
	now := time.Now().UTC()
	dir := t.TempDir()
	b, err := Open(dir, Limits{NodeRatePerMinute: 2})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustApply(t, b, assertAs("root", now, "e1"))
	mustApply(t, b, assertAs("root", now.Add(time.Second), "e2"))
	refusedOp := assertAs("root", now.Add(2*time.Second), "e3")
	reason, refused := rejectReason(t, b, refusedOp)
	if !refused || reason != ReasonRateLimited {
		t.Fatalf("the third move = %q, %v; want %q", reason, refused, ReasonRateLimited)
	}

	ops, err := b.Ops(ctxOf(t))
	if err != nil {
		t.Fatalf("Ops: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("the log holds %d ops, want exactly the two moves that were accepted", len(ops))
	}
	refusedID := DeriveID(refusedOp)
	for _, op := range ops {
		if op.ID == refusedID {
			t.Fatalf("a refused move is in the log: %+v", op)
		}
	}

	live, err := b.Snapshot(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	replayed := Fold(ops)
	if replayed.Applied != live.Applied || len(replayed.Nodes) != len(live.Nodes) {
		t.Fatalf("replayed applied/nodes = %d/%d, live = %d/%d", replayed.Applied, len(replayed.Nodes), live.Applied, len(live.Nodes))
	}
	if got, want := replayed.Nodes["root"].State, live.Nodes["root"].State; got != want {
		t.Fatalf("replayed root = %s, live root = %s", got, want)
	}
	again, err := Open(dir, Limits{NodeRatePerMinute: 2})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reopened, err := again.Snapshot(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("reopened Snapshot: %v", err)
	}
	if reopened.Applied != live.Applied || reopened.Nodes["root"].State != live.Nodes["root"].State {
		t.Fatalf("a fresh handle read applied=%d root=%s, want applied=%d root=%s",
			reopened.Applied, reopened.Nodes["root"].State, live.Applied, live.Nodes["root"].State)
	}
}
