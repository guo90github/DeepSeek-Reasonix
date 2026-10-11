package agent

import (
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func twoReadCalls() []provider.ToolCall {
	return []provider.ToolCall{
		{ID: "a", Name: "read_file", Arguments: `{"path":"note.txt"}`},
		{ID: "b", Name: "read_file", Arguments: `{"path":"other.txt"}`},
	}
}

// TestFanoutNudgeWaitsForTheCheaperFix pins the ordering an earlier version got
// wrong: a long turn whose calls were never batched is not offered sub-agents
// before the batching hint has had its say.
func TestFanoutNudgeWaitsForTheCheaperFix(t *testing.T) {
	a := &Agent{}
	a.turn.budget.rounds = fanoutNudgeMinRounds
	read := []toolOutcome{{output: "note"}}

	for round := 1; round <= batchNudgeStreak; round++ {
		iv := a.applyBatchNudge(oneReadCall(), read, 0)
		if strings.Contains(iv.guidance, "fan-out") {
			t.Fatalf("round %d skipped the cheaper batching hint: %q", round, iv.guidance)
		}
	}
	iv := a.applyBatchNudge(oneReadCall(), read, 0)
	if !strings.Contains(iv.guidance, "Host fan-out hint") || !strings.Contains(iv.guidance, "parallel_tasks") {
		t.Fatalf("fan-out was never offered after the batching hint: %q", iv.guidance)
	}
	if again := a.applyBatchNudge(oneReadCall(), read, 0); strings.Contains(again.guidance, "fan-out") {
		t.Fatalf("the fan-out hint repeated within one turn: %q", again.guidance)
	}
}

// TestFanoutNudgeFollowsACallThatAlreadyBatched: a model that batches its calls
// has the cheap fix in use, so the long-turn hint follows on that very round.
func TestFanoutNudgeFollowsACallThatAlreadyBatched(t *testing.T) {
	a := &Agent{}
	read := []toolOutcome{{output: "note"}, {output: "note"}}
	a.turn.budget.rounds = fanoutNudgeMinRounds

	iv := a.applyBatchNudge(twoReadCalls(), read, 0)
	if !strings.Contains(iv.guidance, "Host fan-out hint") {
		t.Fatalf("a batched long turn was not offered fan-out: %q", iv.guidance)
	}
	if again := a.applyBatchNudge(twoReadCalls(), read, 0); strings.Contains(again.guidance, "fan-out") {
		t.Fatalf("the fan-out hint repeated within one turn: %q", again.guidance)
	}
}

// TestFanoutNudgeStaysQuietOnAShortTurn keeps the expensive hint off turns that
// still have the cheap fix ahead of them.
func TestFanoutNudgeStaysQuietOnAShortTurn(t *testing.T) {
	a := &Agent{}
	a.turn.budget.rounds = fanoutNudgeMinRounds - 1
	read := []toolOutcome{{output: "note"}, {output: "note"}}

	a.applyBatchNudge(twoReadCalls(), read, 0)
	if iv := a.applyBatchNudge(twoReadCalls(), read, 0); strings.Contains(iv.guidance, "fan-out") {
		t.Fatalf("a short turn was offered fan-out: %q", iv.guidance)
	}
}

// TestFanoutNudgeEscalatesWhenTheCheapFixIsIgnored: a model that keeps sending
// single calls after the batching hint has spoken has shown the cheap fix is not
// coming, so the escalation does not wait for the long-turn round count.
func TestFanoutNudgeEscalatesWhenTheCheapFixIsIgnored(t *testing.T) {
	a := &Agent{}
	a.turn.budget.rounds = batchNudgeStreak + fanoutIgnoredFixRounds
	read := []toolOutcome{{output: "note"}}

	var last intervention
	for round := 1; round <= batchNudgeStreak+fanoutIgnoredFixRounds; round++ {
		last = a.applyBatchNudge(oneReadCall(), read, 0)
	}
	if !strings.Contains(last.guidance, "Host fan-out hint") {
		t.Fatalf("an ignored cheap fix never escalated to fan-out: %q", last.guidance)
	}
}

// TestFanoutNudgeHoldsUntilTheCheapFixWasIgnored pins the other side of that
// escalation: before the second patience window a turn shorter than the long
// gate still hears nothing, even though the batching hint already spoke.
func TestFanoutNudgeHoldsUntilTheCheapFixWasIgnored(t *testing.T) {
	a := &Agent{}
	a.turn.budget.rounds = batchNudgeStreak + fanoutIgnoredFixRounds - 1
	read := []toolOutcome{{output: "note"}}

	for round := 1; round <= batchNudgeStreak+fanoutIgnoredFixRounds-1; round++ {
		if iv := a.applyBatchNudge(oneReadCall(), read, 0); strings.Contains(iv.guidance, "fan-out") {
			t.Fatalf("round %d offered fan-out before the cheap fix was ignored: %q", round, iv.guidance)
		}
	}
}

// TestFanoutNudgeStaysQuietOffTheReadOnlyPath keeps it away from rounds that
// touch state or fail: sub-agents on a mutating turn would race each other, and
// a stuck round belongs to the guards.
func TestFanoutNudgeStaysQuietOffTheReadOnlyPath(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome toolOutcome
	}{
		{name: "writer", outcome: toolOutcome{output: "done", resolved: true, resolvedReadOnly: false}},
		{name: "failure", outcome: toolOutcome{errMsg: "boom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{}
			a.turn.budget.rounds = fanoutNudgeMinRounds
			a.turn.loop.markBatchingInUse()
			iv := a.applyBatchNudge(oneReadCall(), []toolOutcome{tc.outcome}, 0)
			if iv.fired() {
				t.Fatalf("a %s round was offered a hint: %q", tc.name, iv.guidance)
			}
		})
	}
}
