package control

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

func busEvidence(kind, ref string) board.Evidence {
	return board.Evidence{Kind: kind, Ref: ref}
}

func TestHearingRefutesAnOutweighedAssertionAndBlocksTheNode(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	orchestrator := newAgentBusTalkController(t, dir, "orchestrator")
	bob := newAgentBusTalkController(t, dir, "bob")
	alice := newAgentBusTalkController(t, dir, "alice")
	orchestrator.SetAgentBusHearingLimits(agentbus.HearingLimits{MaxRounds: 2, RoundTTL: time.Minute})

	// bob's claim rests on one reference of his own; nothing checks it.
	if _, err := orchestrator.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbAssert, Node: "design", Actor: "bob",
		Evidence: []board.Evidence{busEvidence("opinion", "bob-says-so")},
	}); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := orchestrator.OpenAgentBusHearing(ctx, "design", []string{"bob", "alice"}); err != nil {
		t.Fatalf("open hearing: %v", err)
	}
	if _, err := bob.AnswerAgentBusHearing(ctx, "design", "it works, trust me", nil); err != nil {
		t.Fatalf("bob answer: %v", err)
	}
	if _, err := alice.AnswerAgentBusHearing(ctx, "design", "it does not", []board.Evidence{
		busEvidence("verification", "go test ./internal/agentbus/..."),
		busEvidence("diff", "revert.patch"),
	}); err != nil {
		t.Fatalf("alice answer: %v", err)
	}

	record, err := orchestrator.SettleAgentBusHearing(ctx, "design")
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if record.Verdict != agentbus.VerdictRefuted {
		t.Fatalf("verdict = %q (%s), want the outweighing refutation", record.Verdict, record.Reason)
	}

	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := state.Nodes["design"].Outcome; got != board.OutcomeBlocked {
		t.Fatalf("outcome = %q, want the refuted node blocked", got)
	}
	if state.Nodes["design"].Ready(state) {
		t.Fatal("a blocked node must not be ready")
	}

	// A verdict is a fact in the log, not a computation this process owns: a fresh
	// reader reaches the same conclusion from the same files (T6-2).
	reader := newAgentBusTalkController(t, dir, "reader")
	hearings, ok := reader.AgentBusHearings(ctx)
	if !ok || len(hearings) != 1 {
		t.Fatalf("hearings = %+v ok=%v, want the recorded deliberation", hearings, ok)
	}
	if hearings[0].Verdict != agentbus.VerdictRefuted || hearings[0].Reason != agentbus.ReasonWeight {
		t.Fatalf("replayed hearing = %+v, want the same verdict and reason", hearings[0])
	}
	log, err := agentbus.OpenHearingLog(dir)
	if err != nil {
		t.Fatalf("open hearing log: %v", err)
	}
	_, read, err := log.Read()
	if err != nil {
		t.Fatalf("read hearing log: %v", err)
	}
	refolded := agentbus.FoldHearings(read.Items)
	if refolded.Hearings["design"].Verdict != hearings[0].Verdict {
		t.Fatalf("folding the same records twice diverged: %+v", refolded.Hearings["design"])
	}
}

func TestASilentParticipantIsWokenToAnswer(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	orchestrator := newAgentBusTalkController(t, dir, "orchestrator")
	orchestrator.SetAgentBusHearingLimits(agentbus.HearingLimits{RoundTTL: time.Minute})
	var woken []agentbus.WakeTarget
	orchestrator.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target)
		return nil
	})

	log, err := agentbus.OpenHearingLog(dir)
	if err != nil {
		t.Fatalf("open hearing log: %v", err)
	}
	if _, err := log.Append(ctx, agentbus.HearingRecord{
		Node: "design", Kind: agentbus.HearingOpen, Actor: "orchestrator", Required: []string{"bob"},
		At: time.Now().UTC().Add(-time.Hour),
	}, agentbus.HearingLimits{}); err != nil {
		t.Fatalf("open: %v", err)
	}

	if n := orchestrator.WakeAgentBus(ctx); n != 1 {
		t.Fatalf("woken = %d, want bob woken to answer", n)
	}
	if len(woken) != 1 || woken[0].Participant != "bob" {
		t.Fatalf("woken = %+v, want bob", woken)
	}
	if len(woken[0].Owes) != 1 || woken[0].Owes[0] != "design" {
		t.Fatalf("owes = %v, want the deliberation he has not answered", woken[0].Owes)
	}
	if n := orchestrator.WakeAgentBus(ctx); n != 0 {
		t.Fatalf("an unchanged deliberation must not wake twice, got %d", n)
	}
}

func TestSettlingWithoutAHearingIsRefused(t *testing.T) {
	ctx := context.Background()
	ctrl := newAgentBusTalkController(t, t.TempDir(), "orchestrator")
	if _, err := ctrl.SettleAgentBusHearing(ctx, "nothing-here"); err == nil {
		t.Fatal("there is nothing to weigh")
	}
}
