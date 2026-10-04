package agentbus

import (
	"strings"
	"testing"
	"time"
)

// The first screen has to say which rule closed a deliberation: escalation-quota and
// evidence-weight call for different actions, and the reason otherwise lives only in
// hearings.jsonl (2026-10-05).
func TestObserveNamesTheRuleThatClosedADeliberation(t *testing.T) {
	hearings := NewHearingState()
	if err := ApplyHearing(hearings, openHearing("n1", []string{"alice"}, talkBase), HearingLimits{}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyHearing(hearings, ruleHearing("n1", VerdictUndecided, ReasonQuota, "orchestrator", talkBase.Add(time.Second)), HearingLimits{}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyHearing(hearings, openHearing("n2", []string{"alice"}, talkBase.Add(2*time.Second)), HearingLimits{}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyHearing(hearings, ruleHearing("n2", VerdictEscalate, ReasonEqualWeight, "orchestrator", talkBase.Add(3*time.Second)), HearingLimits{}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyHearing(hearings, openHearing("n3", []string{"alice"}, talkBase.Add(4*time.Second)), HearingLimits{}); err != nil {
		t.Fatal(err)
	}

	digest := Observe(nil, nil, hearings, talkBase, ObserveLimits{})
	details := map[SignalKind]string{}
	for _, s := range digest.Signals {
		details[s.Kind] = s.Detail
	}
	if got := details[SignalUndecided]; !strings.Contains(got, ReasonQuota) {
		t.Fatalf("undecided detail = %q, want the rule that closed it (%s)", got, ReasonQuota)
	}
	if got := details[SignalEscalated]; !strings.Contains(got, ReasonEqualWeight) {
		t.Fatalf("escalated detail = %q, want the rule that closed it (%s)", got, ReasonEqualWeight)
	}
	if got := details[SignalDisputed]; got != "under deliberation" {
		t.Fatalf("an open deliberation reads %q, want it unchanged: nothing closed it yet", got)
	}
}
