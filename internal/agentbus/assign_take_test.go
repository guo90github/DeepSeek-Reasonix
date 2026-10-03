package agentbus

import (
	"context"
	"testing"

	"reasonix/internal/agentbus/board"
)

func assignedStepState() *board.State {
	return board.Fold([]board.Op{
		assertOp("design", "alice"),
		requireOp("design", "step"),
		{Verb: board.VerbAssign, Node: "step", Actor: "alice", Assignee: "bob"},
	})
}

func TestWakeTargetsAddressAnAssignedStepToItsAssignee(t *testing.T) {
	targets := WakeTargets(WakeInput{State: assignedStepState()})
	if len(targets) != 1 || targets[0].Participant != "bob" {
		t.Fatalf("targets = %+v, want only bob, the assignee", targets)
	}
	if len(targets[0].Ready) != 1 || targets[0].Ready[0] != "step" {
		t.Fatalf("ready = %v, want the assigned step", targets[0].Ready)
	}
}

func TestTakeLeavesWorkAssignedToAnotherParticipantParked(t *testing.T) {
	ctx := context.Background()
	log, err := OpenQueueLog(t.TempDir())
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	ledger := NewLedger(BudgetLimits{})
	st := assignedStepState()
	if _, _, err := log.Enqueue(ctx, QueueEntry{Node: "step", Participant: "bob"}, QueueLimits{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	taken, err := TakeRanked(ctx, log, ledger, st, "alice", 1, nil, QueueLimits{})
	if err != nil || len(taken) != 0 {
		t.Fatalf("alice took %+v (%v), want the step left for its assignee", taken, err)
	}
	queue, _, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if queue.Depth() != 1 {
		t.Fatalf("queue depth = %d, want the assigned step still parked", queue.Depth())
	}

	taken, err = TakeRanked(ctx, log, ledger, st, "bob", 1, nil, QueueLimits{})
	if err != nil || len(taken) != 1 || taken[0].Entry.Node != "step" {
		t.Fatalf("bob took %+v (%v), want the step he was assigned", taken, err)
	}
}
