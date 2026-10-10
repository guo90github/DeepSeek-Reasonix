package agent

import (
	"strings"
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

func oneReadCall() []provider.ToolCall {
	return []provider.ToolCall{{ID: "r", Name: "read_file", Arguments: `{"path":"note.txt"}`}}
}

// TestBatchNudgeFiresOnceAfterAStreakOfSingleReads pins the threshold and the
// once-per-turn latch: the host hints after the run of single-call rounds makes
// the waste visible, and does not repeat within the same turn.
func TestBatchNudgeFiresOnceAfterAStreakOfSingleReads(t *testing.T) {
	a := &Agent{}
	read := []toolOutcome{{output: "note"}}

	for round := 1; round < batchNudgeStreak; round++ {
		if iv := a.applyBatchNudge(oneReadCall(), read, 0); iv.fired() {
			t.Fatalf("round %d nudged too early: %q", round, iv.guidance)
		}
	}
	iv := a.applyBatchNudge(oneReadCall(), read, 0)
	if !iv.fired() || !strings.Contains(iv.guidance, "Host batching hint") {
		t.Fatalf("the streak did not produce a batching hint: %q", iv.guidance)
	}
	if again := a.applyBatchNudge(oneReadCall(), read, 0); again.guidance != "" {
		t.Fatalf("the hint repeated within one turn: %q", again.guidance)
	}
}

// TestBatchNudgeResetsOnAWrite keeps the hint away from rounds that touch state:
// a writer ends the read-only run, whatever it succeeded at.
func TestBatchNudgeResetsOnAWrite(t *testing.T) {
	a := &Agent{}
	read := []toolOutcome{{output: "note"}}
	write := []toolOutcome{{output: "done", resolved: true, resolvedReadOnly: false}}

	a.applyBatchNudge(oneReadCall(), read, 0)
	a.applyBatchNudge(oneReadCall(), read, 0)
	a.applyBatchNudge(oneReadCall(), write, 0)
	for round := 1; round < batchNudgeStreak; round++ {
		if iv := a.applyBatchNudge(oneReadCall(), read, 0); iv.fired() {
			t.Fatalf("a write did not reset the streak (round %d): %q", round, iv.guidance)
		}
	}
}

// TestBatchNudgeResetsOnAMultiCallRound: a round that already batched its calls
// is the behavior the hint asks for, so it clears the run instead of extending
// it.
func TestBatchNudgeResetsOnAMultiCallRound(t *testing.T) {
	a := &Agent{}
	read := []toolOutcome{{output: "note"}}
	batched := []provider.ToolCall{
		{ID: "a", Name: "read_file", Arguments: `{"path":"note.txt"}`},
		{ID: "b", Name: "read_file", Arguments: `{"path":"other.txt"}`},
	}

	a.applyBatchNudge(oneReadCall(), read, 0)
	a.applyBatchNudge(oneReadCall(), read, 0)
	a.applyBatchNudge(batched, []toolOutcome{{output: "note"}, {output: "note"}}, 0)
	for round := 1; round < batchNudgeStreak; round++ {
		if iv := a.applyBatchNudge(oneReadCall(), read, 0); iv.fired() {
			t.Fatalf("a batched round did not reset the streak (round %d): %q", round, iv.guidance)
		}
	}
}

// TestBatchNudgeResetsOnFailures: a blocked or failed call means the round has a
// real problem, which belongs to the loop guards rather than to a batching hint.
func TestBatchNudgeResetsOnFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome toolOutcome
	}{
		{name: "failure", outcome: toolOutcome{errMsg: "boom"}},
		{name: "blocker", outcome: toolOutcome{blocked: true, errMsg: "blocked by hook"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{}
			read := []toolOutcome{{output: "note"}}
			a.applyBatchNudge(oneReadCall(), read, 0)
			a.applyBatchNudge(oneReadCall(), read, 0)
			a.applyBatchNudge(oneReadCall(), []toolOutcome{tc.outcome}, 0)
			for round := 1; round < batchNudgeStreak; round++ {
				if iv := a.applyBatchNudge(oneReadCall(), read, 0); iv.fired() {
					t.Fatalf("a %s did not reset the streak (round %d): %q", tc.name, round, iv.guidance)
				}
			}
		})
	}
}

// TestBatchNudgeStaysQuietOnAZeroGainRun keeps the hint out of a loop: rounds
// that earn no successful receipt are the progress guard's jurisdiction, and
// asking a stuck model to batch more calls would only multiply the waste.
func TestBatchNudgeStaysQuietOnAZeroGainRun(t *testing.T) {
	a := &Agent{task: taskRuntime{ledger: evidence.NewLedger()}}
	read := []toolOutcome{{output: "note"}}

	for round := 1; round <= batchNudgeStreak+1; round++ {
		if iv := a.applyBatchNudge(oneReadCall(), read, 0); iv.fired() {
			t.Fatalf("round %d hinted on a zero-gain run: %q", round, iv.guidance)
		}
	}
}
