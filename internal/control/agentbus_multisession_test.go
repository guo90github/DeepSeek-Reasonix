package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
)

// TestTwoSessionsShareOneBoardWithoutSeeingEachOther is T4-1: two sessions write
// the same board (one file, two participants) and still see only their own work.
func TestTwoSessionsShareOneBoardWithoutSeeingEachOther(t *testing.T) {
	boardDir := t.TempDir()
	alice := New(Options{SessionDir: t.TempDir(), Label: "alice", Sink: event.Discard})
	bob := New(Options{SessionDir: t.TempDir(), Label: "bob", Sink: event.Discard})
	alice.SetAgentBus(boardDir, "alice")
	bob.SetAgentBus(boardDir, "bob")

	bg := context.Background()
	if _, err := alice.ApplyAgentBusOp(bg, busAssert("alice-task", "alice")); err != nil {
		t.Fatalf("alice apply: %v", err)
	}
	if _, err := bob.ApplyAgentBusOp(bg, busAssert("bob-task", "bob")); err != nil {
		t.Fatalf("bob apply: %v", err)
	}

	// One board, two writers: a reader that is neither of them folds both nodes out of the
	// same file (the whole-board row projection this used to read is gone — G8).
	brd, err := board.Open(boardDir)
	if err != nil {
		t.Fatalf("open the shared board: %v", err)
	}
	state, err := brd.Snapshot(bg, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot the shared board: %v", err)
	}
	for _, id := range []string{"alice-task", "bob-task"} {
		if state.Nodes[id] == nil {
			t.Fatalf("shared board = %+v, want both sessions' nodes", state.Nodes)
		}
	}

	aliceBlock := alice.agentBusTurnBlock()
	if !strings.Contains(aliceBlock, "node id=alice-task") {
		t.Fatalf("alice lost her own node:\n%s", aliceBlock)
	}
	if strings.Contains(aliceBlock, "node id=bob-task") {
		t.Fatalf("bob's node leaked into alice's turn:\n%s", aliceBlock)
	}
	bobBlock := bob.agentBusTurnBlock()
	if !strings.Contains(bobBlock, "node id=bob-task") {
		t.Fatalf("bob lost his own node:\n%s", bobBlock)
	}
	if strings.Contains(bobBlock, "node id=alice-task") {
		t.Fatalf("alice's node leaked into bob's turn:\n%s", bobBlock)
	}
}
