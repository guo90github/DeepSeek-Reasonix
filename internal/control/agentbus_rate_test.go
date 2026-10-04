package control

import (
	"context"
	"testing"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/config"
	"reasonix/internal/event"
)

// moveOp is one assertion with its own evidence, so each call is a distinct move rather than
// a replayed intent.
func moveOp(node, ref string) board.Op {
	return board.Op{
		Verb: board.VerbAssert, Node: node, Actor: "alice",
		Evidence: []board.Evidence{{Kind: "test", Ref: ref}},
	}
}

func rateController(t *testing.T, perMinute int) *Controller {
	t.Helper()
	boardDir := t.TempDir()
	c := New(Options{SessionDir: t.TempDir(), Label: "alice", Sink: event.Discard})
	c.SetAgentBus(boardDir, "alice")
	c.SetAgentBusNodeRate(AgentBusNodeRate(config.AgentBusConfig{NodeRatePerMinute: perMinute}))
	return c
}

// §S5's fourth hard limit reaches the board through the command face, and a refused move comes
// back typed — the caller reports which ceiling bit instead of guessing — and is counted,
// because a refused move leaves nothing else behind (G3).
func TestTheNodeRateCeilingRefusesAMoveThroughTheCommandFace(t *testing.T) {
	c := rateController(t, 1)
	bg := context.Background()
	if _, err := c.ApplyAgentBusOp(bg, moveOp("hot", "e1")); err != nil {
		t.Fatalf("the first move: %v", err)
	}
	before := AgentBusNodeRateRefusals()
	_, err := c.ApplyAgentBusOp(bg, moveOp("hot", "e2"))
	if err == nil {
		t.Fatal("the second move on one node was accepted, want the ceiling to refuse it")
	}
	reason, isReject := board.IsReject(err)
	if !isReject || reason != board.ReasonRateLimited {
		t.Fatalf("the second move = %v (reason %q, typed %v), want %q", err, reason, isReject, board.ReasonRateLimited)
	}
	after := AgentBusNodeRateRefusals()
	if after.Count != before.Count+1 {
		t.Fatalf("refusals = %d, want %d (the refusal must be counted)", after.Count, before.Count+1)
	}
	if after.Last != "hot" {
		t.Fatalf("the last refusal names %q, want the node it turned down", after.Last)
	}
}

func TestNoNodeRateCeilingOnTheCommandFaceAcceptsEveryMove(t *testing.T) {
	c := rateController(t, 0)
	bg := context.Background()
	for _, ref := range []string{"e1", "e2", "e3"} {
		if _, err := c.ApplyAgentBusOp(bg, moveOp("hot", ref)); err != nil {
			t.Fatalf("move %s with no ceiling: %v", ref, err)
		}
	}
}

func TestAgentBusNodeRateMapsTheOperatorsKnob(t *testing.T) {
	if got := AgentBusNodeRate(config.AgentBusConfig{}); got != (board.Limits{}) {
		t.Fatalf("an absent knob = %+v, want the ceiling off", got)
	}
	if got := AgentBusNodeRate(config.AgentBusConfig{NodeRatePerMinute: 5}); got.NodeRatePerMinute != 5 {
		t.Fatalf("NodeRatePerMinute = %d, want the operator's 5", got.NodeRatePerMinute)
	}
}
