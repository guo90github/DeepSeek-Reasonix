package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func TestTakeStopsAtTheHostCeilingAndLeavesTheRestParked(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	log, err := OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, node := range []string{"n1", "n2"} {
		if _, _, err := log.Enqueue(ctx, QueueEntry{Node: node, Subtree: "root"}, QueueLimits{}); err != nil {
			t.Fatalf("enqueue %s: %v", node, err)
		}
	}
	ledger := NewLedger(BudgetLimits{Slots: 1})

	taken, err := Take(ctx, log, ledger, nil, "alice", 1, QueueLimits{})
	if err != nil || len(taken) != 1 || taken[0].Node != "n1" {
		t.Fatalf("take = (%+v, %v), want the oldest entry for the one free slot", taken, err)
	}
	if ledger.SlotsInUse() != 1 {
		t.Fatalf("slots in use = %d, want alice holding the host's only slot", ledger.SlotsInUse())
	}

	// The host is full: the second claimant takes nothing, and the work stays parked
	// rather than failing.
	_, err = Take(ctx, log, ledger, nil, "bob", 1, QueueLimits{})
	if reason, ok := IsBudgetReject(err); !ok || reason != RefuseSlots {
		t.Fatalf("the full host = (%v, %q), want slots_exhausted", ok, reason)
	}
	state, _, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if state.Depth() != 1 || state.Waiting["n2"].Node != "n2" {
		t.Fatalf("queue = %+v, want n2 still waiting", state.Waiting)
	}

	ledger.ReleaseSlot("alice")
	taken, err = Take(ctx, log, ledger, nil, "bob", 1, QueueLimits{})
	if err != nil || len(taken) != 1 || taken[0].Node != "n2" {
		t.Fatalf("after the slot freed = (%+v, %v), want n2 taken", taken, err)
	}
}

func TestParkingNeverTouchesTheBoard(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	op := board.Op{Verb: board.VerbAssert, Node: "n1", Actor: "alice",
		Evidence: []board.Evidence{{Kind: "test", Ref: "test:parking"}}}
	if _, err := brd.Apply(ctx, op); err != nil {
		t.Fatalf("assert: %v", err)
	}
	before, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}

	log, err := OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	if _, _, err := log.Enqueue(ctx, QueueEntry{Node: "n1", Subtree: "root", Participant: "alice"}, QueueLimits{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	ledger := NewLedger(BudgetLimits{Slots: 1})
	if _, err := Take(ctx, log, ledger, nil, "alice", 1, QueueLimits{}); err != nil {
		t.Fatalf("take: %v", err)
	}

	after, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("board ops = %d, want %d: parking and claiming must not touch the board's scene", len(after), len(before))
	}
	state, err := brd.Snapshot(ctx, talkBase)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if state.Nodes["n1"].State != board.StateOpen {
		t.Fatalf("node = %+v, want it exactly as it was left", state.Nodes["n1"])
	}
}

func TestTakeRankedChoosesByAdviceWithoutMovingTheQueue(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	log, err := OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st := chainState(t)
	// root arrived last: picking it proves the advice chose, not the queue.
	for _, entry := range []QueueEntry{
		{Node: "leaf", Subtree: "root", EnqueuedAt: talkBase.Add(3 * time.Second)},
		{Node: "mid", Subtree: "root", EnqueuedAt: talkBase.Add(2 * time.Second)},
		{Node: "root", Subtree: "root", EnqueuedAt: talkBase.Add(time.Second)},
	} {
		if _, _, err := log.Enqueue(ctx, entry, QueueLimits{}); err != nil {
			t.Fatalf("enqueue %s: %v", entry.Node, err)
		}
	}

	ledger := NewLedger(BudgetLimits{Slots: 1})
	taken, err := TakeRanked(ctx, log, ledger, st, "alice", 1, nil, QueueLimits{})
	if err != nil || len(taken) != 1 || taken[0].Entry.Node != "root" {
		t.Fatalf("take = (%+v, %v), want the most unblocking entry", taken, err)
	}

	queue, _, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Visible order is untouched: the advice picked, it did not reorder the queue.
	next := queue.Next(0)
	if len(next) != 2 || next[0].Node != "mid" || next[1].Node != "leaf" {
		t.Fatalf("queue order = %+v, want arrival order with root removed", next)
	}
}
