package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// writeAs lands one op as an explicit participant, which is how a test stands in for a session
// that has since gone away: the board records who wrote, not whether they are still here.
func writeAs(t *testing.T, ctrl interface {
	ApplyAgentBusOp(context.Context, board.Op) (board.Receipt, error)
}, op board.Op) {
	t.Helper()
	if _, err := ctrl.ApplyAgentBusOp(context.Background(), op); err != nil {
		t.Fatalf("apply %s %s as %s: %v", op.Verb, op.Node, op.Actor, err)
	}
}

// A board outlives the sessions that wrote to it. The panel says which cards nobody here can act
// on, so history does not read like a call to work — and the participants of this host stay live.
func TestALeftoverSubtreeIsMarkedForTheParticipantsThatLeft(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("join: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "leftover"}}
	// Observe draws a card only for a subtree that needs attention, so both steps are made
	// disputed. One belongs to a participant that has left; the other is this session's own.
	writeAs(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "ghost-work", Actor: "ghost", Evidence: evidence})
	writeAs(t, ctrl, board.Op{Verb: board.VerbRefute, Node: "ghost-work", Actor: "me", Reason: "cannot check it"})
	writeAs(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "mine", Actor: ctrl.AgentBusParticipant(), Evidence: evidence})
	writeAs(t, ctrl, board.Op{Verb: board.VerbRefute, Node: "mine", Actor: "ghost", Reason: "cannot check it"})

	view, err := app.AgentBusBriefing()
	if err != nil {
		t.Fatalf("briefing: %v", err)
	}
	leftover := map[string]bool{}
	for _, card := range view.Cards {
		leftover[card.Subtree] = card.Leftover
	}
	if _, seen := leftover["ghost-work"]; !seen {
		t.Fatalf("cards = %+v, want a card for the disputed step nobody here can act on", view.Cards)
	}
	if !leftover["ghost-work"] {
		t.Fatalf("cards = %+v, want the departed participant's step marked leftover", view.Cards)
	}
	if _, seen := leftover["mine"]; !seen {
		t.Fatalf("cards = %+v, want a card for the disputed step this session owns", view.Cards)
	}
	if leftover["mine"] {
		t.Fatalf("cards = %+v, want the session's own step left alone", view.Cards)
	}
}

// Retiring runs the board's own two-step protocol, as the human: an abandon carrying evidence, then
// the decision that closes it. No side door into the log, and the node really ends up abandoned.
func TestRetiringALeftoverSubtreeAbandonsItsSteps(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("join: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "leftover"}}
	writeAs(t, ctrl, board.Op{
		Verb: board.VerbAssert, Node: "ghost-deliverable", Actor: "ghost", Evidence: evidence,
	})
	writeAs(t, ctrl, board.Op{
		Verb: board.VerbRequire, Node: "ghost-deliverable", Actor: "ghost",
		Dep: &board.NodeSpec{ID: "ghost-block"}, Evidence: evidence,
	})

	// The subtree a node belongs to is named by SubtreeRoot: walking dependencies upwards, so these
	// two steps share the id of the one nothing depends on.
	answer, err := app.AgentBusRetireSubtree(AgentBusRetireArgs{Subtree: "ghost-block"})
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if !strings.Contains(answer, "ghost-block") || !strings.Contains(answer, "2") {
		t.Fatalf("answer = %q, want it to say what it dropped", answer)
	}

	brd, err := board.Open(ctrl.AgentBusDir())
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for _, node := range []string{"ghost-deliverable", "ghost-block"} {
		n := state.Nodes[node]
		if n == nil || n.State != board.StateAbandoned {
			t.Fatalf("node %q = %+v, want abandoned", node, n)
		}
	}
	// The retirement is recorded in the log, not only in the answer: an abandon and a decision per
	// node, and both written as the human driving the panel rather than as some system actor.
	ops, err := brd.Ops(context.Background())
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	verbs, mine := map[string]int{}, 0
	for _, op := range ops {
		verbs[string(op.Verb)]++
		if op.Verb == board.VerbAbandon && op.Actor == ctrl.AgentBusParticipant() {
			mine++
		}
	}
	if verbs["abandon"] != 2 || verbs["decide"] < 2 || mine != 2 {
		t.Fatalf("ops = %+v with %d of mine, want an abandon and a decision per node, both as the human", verbs, mine)
	}
}
