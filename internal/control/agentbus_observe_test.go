package control

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
)

func signalNodesOf(briefing agentbus.Briefing, kind agentbus.SignalKind) []string {
	out := []string{}
	for _, signal := range briefing.Signals {
		if signal.Kind == kind {
			out = append(out, signal.Node)
		}
	}
	return out
}

func TestBriefingFoldsOnlyWhatNeedsAttention(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ctrl := newAgentBusTalkController(t, dir, "orchestrator")

	if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	// A real claim whose lease lapses while the test watches.
	if _, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "design", Actor: "alice",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(50 * time.Millisecond),
	}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Work parked behind a full host: normal, so it counts but raises no signal.
	queueLog, err := agentbus.OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	if _, _, err := queueLog.Enqueue(ctx, agentbus.QueueEntry{Node: "later", Subtree: "design"}, agentbus.QueueLimits{}); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := ctrl.OpenAgentBusHearing(ctx, "design", []string{"alice"}); err != nil {
		t.Fatalf("open hearing: %v", err)
	}

	time.Sleep(120 * time.Millisecond)
	briefing, ok := ctrl.AgentBusBriefing(time.Now().UTC())
	if !ok {
		t.Fatal("an enrolled session must produce a briefing")
	}
	if len(briefing.Cards) != 1 {
		t.Fatalf("cards = %+v, want the one subtree with something wrong", briefing.Cards)
	}
	card := briefing.Cards[0]
	if card.Subtree != "design" || card.AtWork != 1 || card.Parked != 1 {
		t.Fatalf("card = %+v, want the design subtree folded with its counts", card)
	}
	if card.Stalled != 1 || card.Disputed != 1 || card.Worst != agentbus.SignalStalled {
		t.Fatalf("card = %+v, want the lapsed lease and the open deliberation", card)
	}
	if got := signalNodesOf(briefing, agentbus.SignalStalled); len(got) != 1 || got[0] != "design" {
		t.Fatalf("stalls = %v, want the lapsed lease reported", got)
	}
	if got := signalNodesOf(briefing, agentbus.SignalDisputed); len(got) != 1 {
		t.Fatalf("disputes = %v, want the open deliberation reported", got)
	}
}

func TestBriefingIsUnavailableToASessionOffTheBoard(t *testing.T) {
	unenrolled := New(Options{SessionDir: t.TempDir(), Sink: event.Discard})
	if _, ok := unenrolled.AgentBusBriefing(time.Now().UTC()); ok {
		t.Fatal("a session off the board has nothing to show")
	}
	if got := unenrolled.AgentBusParticipant(); got != "" {
		t.Fatalf("participant = %q, want empty for routing purposes", got)
	}
}

func TestParticipantIDIsReadableForHostRouting(t *testing.T) {
	ctrl := newAgentBusTalkController(t, t.TempDir(), "orchestrator")
	if got := ctrl.AgentBusParticipant(); got != "orchestrator" {
		t.Fatalf("participant = %q, want the explicit id", got)
	}
	ctrl.SetAgentBusObserveLimits(agentbus.ObserveLimits{MaxCards: 1, MaxSignals: 1})
	if _, ok := ctrl.AgentBusBriefing(time.Now().UTC()); !ok {
		t.Fatal("a wired session keeps producing a briefing under custom limits")
	}
}
