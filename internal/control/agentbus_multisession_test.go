package control

import (
	"context"
	"strings"
	"testing"
	"time"

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

	rows, ok := alice.AgentBusTasks(time.Now().UTC())
	if !ok || len(rows) != 2 {
		t.Fatalf("board rows = %+v ok=%v, want both sessions' nodes on one board", rows, ok)
	}
	if rows[0].ID != "alice-task" || rows[1].ID != "bob-task" {
		t.Fatalf("rows are not id-sorted: %+v", rows)
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
