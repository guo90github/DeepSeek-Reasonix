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

// The chain nobody clicks: a refutation lands, the session that owns the contested
// step opens the deliberation itself, the side that must answer hears about it, and
// the verdict lands. A deliberation gets its round window first (that is what the TTL
// is), so the test opens it with a window that has effectively lapsed — the point here
// is that the *writer* asks for the wake, which no production path did before (G1).
func TestTheRefuteToVerdictChainRunsWithoutAnyoneClickingAnything(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bob := newAgentBusTalkController(t, dir, "bob")
	alice := newAgentBusTalkController(t, dir, "alice")
	alice.SetAgentBusHearingLimits(agentbus.HearingLimits{RoundTTL: time.Nanosecond})
	var woken []agentbus.WakeTarget
	alice.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target)
		return nil
	})

	// bob's step rests on his own word, and he is the one holding it.
	if _, err := bob.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbAssert, Node: "design", Actor: "bob",
		Evidence: []board.Evidence{busEvidence("opinion", "bob-says-so")},
	}); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := bob.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "design", Actor: "bob",
		Bounds:   &board.Bounds{Steps: 2},
		Deadline: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// alice challenges it, and says why: a refutation with a reason is what this
	// board can hold without a human reading it.
	if _, err := alice.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbRefute, Node: "design", Actor: "alice",
		Reason: "the reference behind this is not something I can check",
	}); err != nil {
		t.Fatalf("refute: %v", err)
	}

	woken = nil
	if _, err := alice.OpenAgentBusHearing(ctx, "design", nil); err != nil {
		t.Fatalf("open the hearing on her own refutation: %v", err)
	}
	if len(woken) == 0 {
		// The opener does not wake herself, so the only target here is the owner: the
		// write path is what has to ask, and no production path asked before this (G1).
		_, input, probeErr := alice.agentBusWakeSnapshot(ctx)
		t.Fatalf("opening a deliberation woke nobody (probe err=%v, targets=%+v)", probeErr, agentbus.WakeTargets(input))
	}
	for _, target := range woken {
		if target.Participant != "bob" {
			t.Fatalf("woken %+v, want the owner who still owes an answer (the opener does not wake herself)", woken)
		}
		if len(target.Owes) != 1 || target.Owes[0] != "design" {
			t.Fatalf("owes = %v, want the deliberation he has not answered", target.Owes)
		}
	}

	// He answers on his own word; she answers with what someone can go and check.
	if _, err := bob.AnswerAgentBusHearing(ctx, "design", "it works, trust me", nil); err != nil {
		t.Fatalf("bob answer: %v", err)
	}
	if _, err := alice.AnswerAgentBusHearing(ctx, "design", "it does not hold", []board.Evidence{
		busEvidence("test", "go test ./internal/agentbus/..."),
		busEvidence("diff", "revert.patch"),
	}); err != nil {
		t.Fatalf("alice answer: %v", err)
	}
	record, err := alice.SettleAgentBusHearing(ctx, "design")
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
		t.Fatalf("outcome = %q, want the refuted step blocked", got)
	}

	// The ending is a fact in the files, not a state this process kept: a reader that
	// was never part of the chain folds the same verdict.
	reader := newAgentBusTalkController(t, dir, "reader")
	hearings, ok := reader.AgentBusHearings(ctx)
	if !ok || len(hearings) != 1 || hearings[0].Verdict != agentbus.VerdictRefuted {
		t.Fatalf("replayed hearings = %+v ok=%v, want the same verdict", hearings, ok)
	}
}
