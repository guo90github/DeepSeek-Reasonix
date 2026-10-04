package agentbus

import (
	"context"
	"testing"

	"reasonix/internal/agentbus/board"
)

// F3: a node the board has already finished is not waiting for a slot, so the queue must not hand
// it out again. That take+claim pair is what grew queue.jsonl one node at a time and produced a
// pair of records every 30s for a node that had been done for hours (measured 2026-10-04).
func TestSettledWorkIsNotTakenFromTheQueue(t *testing.T) {
	ctx := context.Background()
	log, err := OpenQueueLog(t.TempDir())
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	nodes := []string{"done-step", "abandoned-step", "open-step"}
	for _, node := range nodes {
		if _, _, err := log.Enqueue(ctx, QueueEntry{Node: node, Subtree: node}, QueueLimits{}); err != nil {
			t.Fatalf("enqueue %s: %v", node, err)
		}
	}
	st := &board.State{Nodes: map[string]*board.Node{
		"done-step":      {ID: "done-step", State: board.StateDone},
		"abandoned-step": {ID: "abandoned-step", State: board.StateAbandoned},
		"open-step":      {ID: "open-step", State: board.StateOpen},
	}}

	taken, err := TakeRanked(ctx, log, NewLedger(BudgetLimits{}), st, "alice", 5, nil, QueueLimits{})
	if err != nil {
		t.Fatalf("take ranked: %v", err)
	}
	if len(taken) != 1 || taken[0].Entry.Node != "open-step" {
		t.Fatalf("took %v, want only the open step", taken)
	}
}
