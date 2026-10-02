package control

import (
	"testing"

	"reasonix/internal/agent"
)

// TestGoalPauseMappingKeepsTheAxis is the case that would have caught the bug this
// change fixes: the mapping used to report every budget pause as spend, so a token
// stop was recorded under the wrong cause and `budget_tokens` was never written.
func TestGoalPauseMappingKeepsTheAxis(t *testing.T) {
	cases := []struct {
		name      string
		pause     agent.RunPauseInfo
		wantCause string
		wantOK    bool
	}{
		{
			"a token stop says tokens",
			agent.RunPauseInfo{Kind: "task_budget", Key: "token", HostOwned: true, Reason: "task used 10 tokens"},
			stopCauseBudgetTokens, true,
		},
		{
			"a spend stop says spend",
			agent.RunPauseInfo{Kind: "task_budget", Key: "cost", HostOwned: true, Reason: "spend reached its budget"},
			stopCauseBudgetSpend, true,
		},
		{
			"a wall-clock stop stays spend-side until it has a cause of its own",
			agent.RunPauseInfo{Kind: "task_budget", Key: "time", HostOwned: true, Reason: "ran too long"},
			stopCauseBudgetSpend, true,
		},
		{
			"a pause this host does not own is not ours to stop on",
			agent.RunPauseInfo{Kind: "task_budget", Key: "token"},
			"", false,
		},
		{
			"another kind of pause is not a budget stop",
			agent.RunPauseInfo{Kind: "max_steps", HostOwned: true, Key: "token"},
			"", false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cause, reason, ok := goalPauseFromPause(tc.pause)
			if ok != tc.wantOK || cause != tc.wantCause {
				t.Fatalf("mapping = (%q, %v), want (%q, %v)", cause, ok, tc.wantCause, tc.wantOK)
			}
			if tc.wantOK && reason == "" {
				t.Fatal("a stop must say why it stopped")
			}
		})
	}
}

func TestGoalPauseMappingSuppliesAReasonWhenThePauseHasNone(t *testing.T) {
	cause, reason, ok := goalPauseFromPause(agent.RunPauseInfo{Kind: "task_budget", Key: "cost", HostOwned: true})
	if !ok || cause != stopCauseBudgetSpend {
		t.Fatalf("mapping = (%q, %v), want a spend stop", cause, ok)
	}
	if reason == "" {
		t.Fatal("an unexplained pause still needs a reason a human can read")
	}
}
