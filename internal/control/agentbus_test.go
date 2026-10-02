package control

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
)

func newAgentBusTestController(t *testing.T) *Controller {
	t.Helper()
	sessionDir := t.TempDir()
	c := New(Options{SessionDir: sessionDir, Label: "test", Sink: event.Discard})
	c.SetSessionPath(filepath.Join(sessionDir, "session.jsonl"))
	return c
}

func applyBusOps(t *testing.T, dir string, ops ...board.Op) {
	t.Helper()
	b, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	if _, err := b.ApplyAll(context.Background(), ops...); err != nil {
		t.Fatalf("apply ops: %v", err)
	}
}

func busAssert(node, actor string) board.Op {
	return board.Op{
		Verb: board.VerbAssert, Node: node, Actor: actor,
		Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:" + node}},
	}
}

func TestAgentBusIsOptedOutByDefault(t *testing.T) {
	c := newAgentBusTestController(t)
	if block := c.agentBusTurnBlock(); block != "" {
		t.Fatalf("an unwired controller must compose unchanged, got %q", block)
	}
	if _, ok := c.AgentBusView(time.Now().UTC()); ok {
		t.Fatalf("an unwired controller has no view")
	}
}

func TestAgentBusTurnBlockCarriesOnlyMyViewAndAdvancesTheCursor(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTestController(t)
	participant := c.parentSessionID()
	if strings.TrimSpace(participant) == "" {
		t.Skip("controller has no session identity to address on a board")
	}
	applyBusOps(t, dir,
		busAssert("mine", participant),
		busAssert("theirs", "someone-else"),
	)
	c.SetAgentBusDir(dir)

	first := c.agentBusTurnBlock()
	if first == "" {
		t.Fatal("the view must reach the turn once the board is wired")
	}
	if !strings.Contains(first, "node id=mine") {
		t.Fatalf("my own node is missing from the turn block:\n%s", first)
	}
	if strings.Contains(first, "node id=theirs") {
		t.Fatalf("another participant's node leaked into my turn:\n%s", first)
	}
	if !strings.Contains(first, "schema=agentbus-view/1") {
		t.Fatalf("the block must carry the schema header:\n%s", first)
	}

	if second := c.agentBusTurnBlock(); second != "" {
		t.Fatalf("the cursor must suppress a repeat delivery, got %q", second)
	}

	applyBusOps(t, dir, busAssert("fresh", participant))
	third := c.agentBusTurnBlock()
	if !strings.Contains(third, "node id=fresh") {
		t.Fatalf("a new op must arrive next turn:\n%s", third)
	}
	if strings.Contains(third, "node id=mine") {
		t.Fatalf("the delta must not repeat what was already delivered:\n%s", third)
	}

	view, ok := c.AgentBusView(time.Now().UTC())
	if !ok || view.Owned != 2 {
		t.Fatalf("peek view = %+v ok=%v, want two owned nodes", view, ok)
	}
}

func TestSetAgentBusDirEmptyOptsOut(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTestController(t)
	applyBusOps(t, dir, busAssert("mine", c.parentSessionID()))
	c.SetAgentBusDir(dir)
	if c.agentBusTurnBlock() == "" {
		t.Fatal("wired board should produce a block")
	}
	c.SetAgentBusDir("")
	if block := c.agentBusTurnBlock(); block != "" {
		t.Fatalf("opting out must stop the block, got %q", block)
	}
	if _, ok := c.AgentBusView(time.Now().UTC()); ok {
		t.Fatal("opting out must drop the view")
	}
}

func TestUnreadableBoardLeavesTheTurnIntact(t *testing.T) {
	c := newAgentBusTestController(t)
	c.SetAgentBusDir(filepath.Join(t.TempDir(), "missing-as-a-file"))
	if block := c.agentBusTurnBlock(); block != "" {
		t.Fatalf("a board that cannot be read must contribute nothing, got %q", block)
	}
}
