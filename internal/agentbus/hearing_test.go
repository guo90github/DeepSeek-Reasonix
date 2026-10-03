package agentbus

import (
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func openHearing(node string, required []string, at time.Time) HearingRecord {
	return HearingRecord{Node: node, Kind: HearingOpen, Actor: "orchestrator", Required: required, At: at}
}

func answerHearing(node, actor string, at time.Time) HearingRecord {
	return HearingRecord{Node: node, Kind: HearingAnswer, Actor: actor, At: at}
}

func ruleHearing(node, verdict, reason, actor string, at time.Time) HearingRecord {
	return HearingRecord{Node: node, Kind: HearingRule, Actor: actor, Verdict: verdict, Reason: reason, At: at}
}

func TestHearingAsksEveryRequiredParticipantOncePerRound(t *testing.T) {
	lim := HearingLimits{MaxRounds: 2}
	st := NewHearingState()
	if err := ApplyHearing(st, openHearing("n1", []string{"alice", "bob"}, talkBase), lim); err != nil {
		t.Fatalf("open: %v", err)
	}
	h := st.Hearings["n1"]
	if h.Round() != 1 {
		t.Fatalf("round = %d, want 1", h.Round())
	}
	if err := ApplyHearing(st, answerHearing("n1", "alice", talkBase.Add(time.Second)), lim); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if h.Round() != 1 || len(h.AnsweredThisRound()) != 1 {
		t.Fatalf("one answer of two must not close a round: round=%d answered=%v", h.Round(), h.AnsweredThisRound())
	}
	reason, ok := IsHearingReject(ApplyHearing(st, answerHearing("n1", "alice", talkBase.Add(2*time.Second)), lim))
	if !ok || reason != RefuseHearingAnswered {
		t.Fatalf("a second answer in one round = (%v, %q), want already_answered", ok, reason)
	}
	if err := ApplyHearing(st, answerHearing("n1", "bob", talkBase.Add(3*time.Second)), lim); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if h.Round() != 2 {
		t.Fatalf("round = %d, want 2 once everyone has answered", h.Round())
	}
	for i, actor := range []string{"alice", "bob"} {
		if err := ApplyHearing(st, answerHearing("n1", actor, talkBase.Add(time.Duration(4+i)*time.Second)), lim); err != nil {
			t.Fatalf("round two answer: %v", err)
		}
	}
	if h.Round() != 3 {
		t.Fatalf("round = %d, want 3", h.Round())
	}
	reason, ok = IsHearingReject(ApplyHearing(st, answerHearing("n1", "alice", talkBase.Add(9*time.Second)), lim))
	if !ok || reason != RefuseHearingRounds {
		t.Fatalf("answering past the bound = (%v, %q), want rounds_exhausted", ok, reason)
	}
}

func TestHearingRefusesToOpenWithoutAnyoneToAnswer(t *testing.T) {
	st := NewHearingState()
	if reason, ok := IsHearingReject(ApplyHearing(st, openHearing("n1", nil, talkBase), HearingLimits{})); !ok || reason != RefuseHearingNoRequired {
		t.Fatalf("open with no required participants = (%v, %q), want no_required", ok, reason)
	}
	if reason, ok := IsHearingReject(ApplyHearing(st, HearingRecord{Node: "  ", Kind: HearingOpen, Required: []string{"alice"}, At: talkBase}, HearingLimits{})); !ok || reason != RefuseHearingNoNode {
		t.Fatalf("open without a node = (%v, %q), want node_missing", ok, reason)
	}
	if reason, ok := IsHearingReject(ApplyHearing(st, HearingRecord{Node: "n1", Kind: "shout", At: talkBase}, HearingLimits{})); !ok || reason != RefuseHearingKind {
		t.Fatalf("unknown kind = (%v, %q), want unknown_kind", ok, reason)
	}
}

func TestHearingCooldownKeepsAClosedQuestionClosed(t *testing.T) {
	lim := HearingLimits{Cooldown: time.Minute}
	st := NewHearingState()
	if err := ApplyHearing(st, openHearing("n1", []string{"alice"}, talkBase), lim); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := ApplyHearing(st, ruleHearing("n1", VerdictRefuted, ReasonWeight, "orchestrator", talkBase.Add(time.Second)), lim); err != nil {
		t.Fatalf("rule: %v", err)
	}
	reason, ok := IsHearingReject(ApplyHearing(st, openHearing("n1", []string{"alice"}, talkBase.Add(10*time.Second)), lim))
	if !ok || reason != RefuseHearingCooldown {
		t.Fatalf("immediate re-open = (%v, %q), want cooldown", ok, reason)
	}
	if err := ApplyHearing(st, openHearing("n1", []string{"alice"}, talkBase.Add(2*time.Minute)), lim); err != nil {
		t.Fatalf("after the cooldown the question may be asked again: %v", err)
	}
	if !st.Hearings["n1"].Open {
		t.Fatal("the hearing should be open again")
	}
}

func TestHearingSilenceIsVisibleNotDeleted(t *testing.T) {
	lim := HearingLimits{RoundTTL: time.Minute}
	st := NewHearingState()
	if err := ApplyHearing(st, openHearing("n1", []string{"alice", "bob"}, talkBase), lim); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := ApplyHearing(st, answerHearing("n1", "alice", talkBase.Add(10*time.Second)), lim); err != nil {
		t.Fatalf("answer: %v", err)
	}
	h := st.Hearings["n1"]
	if silent := HearingSilent(h, talkBase.Add(30*time.Second), lim); len(silent) != 0 {
		t.Fatalf("silent = %v, want nobody while the round is still open", silent)
	}
	silent := HearingSilent(h, talkBase.Add(2*time.Minute), lim)
	if len(silent) != 1 || silent[0] != "bob" {
		t.Fatalf("silent = %v, want bob who never answered", silent)
	}
	if err := ApplyHearing(st, answerHearing("n1", "bob", talkBase.Add(2*time.Minute)), lim); err != nil {
		t.Fatalf("late answer: %v", err)
	}
	if silent := HearingSilent(h, talkBase.Add(3*time.Minute), lim); len(silent) != 0 {
		t.Fatalf("silent = %v, want nobody: both answered", silent)
	}
}

// A deliberation with no round window has no clock at all: silence is never counted, so
// nobody is ever woken to answer. That is what an operator who set no bounds gets, and it
// is the state the host has to say out loud (AGENT_BUS §11.5.7).
func TestADeliberationWithNoRoundWindowNeverCountsSilence(t *testing.T) {
	st := NewHearingState()
	if err := ApplyHearing(st, openHearing("n1", []string{"alice", "bob"}, talkBase), HearingLimits{}); err != nil {
		t.Fatalf("open: %v", err)
	}
	h := st.Hearings["n1"]
	if silent := HearingSilent(h, talkBase.Add(24*time.Hour), HearingLimits{}); len(silent) != 0 {
		t.Fatalf("silent = %v, want nobody: with no window the round never lapses", silent)
	}
	// The same open deliberation, one bound set: the same moment counts as silence.
	if silent := HearingSilent(h, talkBase.Add(24*time.Hour), HearingLimits{RoundTTL: time.Hour}); len(silent) != 2 {
		t.Fatalf("silent = %v, want both required participants once a window is set", silent)
	}
}

func TestWeighResponseFollowsEvidenceNotVotes(t *testing.T) {
	evidence := func(kind, ref string) board.Evidence { return board.Evidence{Kind: kind, Ref: ref} }
	supported := []board.Evidence{evidence("verification", "go test ./...")}
	opinions := []board.Evidence{
		evidence("opinion", ""), evidence("opinion", ""), evidence("opinion", ""),
		evidence("opinion", ""), evidence("opinion", ""),
	}

	cases := []struct {
		name          string
		claim, refute []board.Evidence
		escalated     int
		quota         int
		wantVerdict   string
		wantReason    string
	}{
		{"an evidenced claim stands", supported, opinions, 0, 2, VerdictStands, ReasonWeight},
		{"five opinions lose to one test", opinions, supported, 0, 2, VerdictRefuted, ReasonWeight},
		{"equal weight goes to a human", supported, []board.Evidence{evidence("diff", "x.patch")}, 0, 2, VerdictEscalate, ReasonEqualWeight},
		{"two bare claims are still equal", opinions, opinions, 1, 2, VerdictEscalate, ReasonEqualWeight},
		{"an exhausted quota closes by rule", opinions, opinions, 2, 2, VerdictUndecided, ReasonQuota},
		{"no quota at all never escalates", opinions, opinions, 0, 0, VerdictUndecided, ReasonQuota},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, reason := WeighResponse(tc.claim, tc.refute, tc.escalated, tc.quota)
			if verdict != tc.wantVerdict || reason != tc.wantReason {
				t.Fatalf("weigh = (%q, %q), want (%q, %q)", verdict, reason, tc.wantVerdict, tc.wantReason)
			}
		})
	}
}

func TestHearingRequiredNamesWhoMustAnswer(t *testing.T) {
	st := &board.State{Nodes: map[string]*board.Node{
		"n1": {
			ID: "n1", Owner: "alice",
			Refutes: []board.Refutation{{Actor: "bob"}, {Actor: "alice"}, {Actor: "carol"}},
		},
	}}
	required := HearingRequired(st, "n1")
	want := []string{"alice", "bob", "carol"}
	if len(required) != len(want) {
		t.Fatalf("required = %v, want %v", required, want)
	}
	for i := range want {
		if required[i] != want[i] {
			t.Fatalf("required = %v, want %v (sorted, deduped)", required, want)
		}
	}
	if got := HearingRequired(st, "missing"); len(got) != 0 {
		t.Fatalf("an unknown node requires nobody: %v", got)
	}
}

func TestWeighResponseClosesTheLogOnceTheQuotaIsSpent(t *testing.T) {
	log, err := OpenHearingLog(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	lim := HearingLimits{EscalationQuota: 1, MaxRounds: 2}
	ctx := t.Context()
	if _, err := log.Append(ctx, openHearing("n1", []string{"alice", "bob"}, talkBase), lim); err != nil {
		t.Fatalf("open: %v", err)
	}
	first, err := log.Weigh(ctx, "n1", nil, nil, "orchestrator", lim)
	if err != nil {
		t.Fatalf("weigh: %v", err)
	}
	if first.Verdict != VerdictEscalate {
		t.Fatalf("first weigh = %q, want an escalation", first.Verdict)
	}
	if _, err := log.Append(ctx, openHearing("n2", []string{"alice"}, talkBase.Add(time.Minute)), lim); err != nil {
		t.Fatalf("open second: %v", err)
	}
	second, err := log.Weigh(ctx, "n2", nil, nil, "orchestrator", lim)
	if err != nil {
		t.Fatalf("weigh second: %v", err)
	}
	if second.Verdict != VerdictUndecided || second.Reason != ReasonQuota {
		t.Fatalf("second weigh = (%q, %q), want undecided-by-rule on the spent quota", second.Verdict, second.Reason)
	}
	state, _, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if state.Escalations != 1 {
		t.Fatalf("escalations = %d, want the one escalation on the ledger", state.Escalations)
	}
	if state.Hearings["n2"].Verdict != VerdictUndecided {
		t.Fatalf("n2 = %+v, want the rule verdict recorded", state.Hearings["n2"])
	}
}
