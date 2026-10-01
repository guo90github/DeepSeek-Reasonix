package control

import (
	"strings"
	"testing"

	"reasonix/internal/agent"
)

// BA2 (docs/70 §2.3): the pre-turn block states recent verdicts and counts only —
// never a prompt, a command, or a path — and it drops the oldest round first when
// the budget is tight.
func TestRenderTurnProgressStatesVerdictsAndCounts(t *testing.T) {
	outcomes := []agent.TurnOutcome{
		{TurnSeq: 1, Verdict: agent.TurnVerdictDelivered},
		{TurnSeq: 2, Verdict: agent.TurnVerdictBlocked, MissingCount: 2},
	}
	block := renderTurnProgress(outcomes, 0)
	for _, want := range []string{"<turn-progress>", "</turn-progress>", "turn 1: delivered", "turn 2: blocked", "2 open obligation"} {
		if !strings.Contains(block, want) {
			t.Fatalf("block missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "turn 3") {
		t.Fatalf("block invented a turn:\n%s", block)
	}

	// Counters the record carries ride along; recovered is named, not counted out.
	detailed := renderTurnProgress([]agent.TurnOutcome{
		{TurnSeq: 7, Verdict: agent.TurnVerdictDelivered, ChangedFiles: 3, Recovered: 1},
	}, 0)
	if !strings.Contains(detailed, "3 file(s) changed") || !strings.Contains(detailed, "recovered") {
		t.Fatalf("detailed block = %q", detailed)
	}

	// An empty verdict is the delivered default.
	if got := renderTurnProgress([]agent.TurnOutcome{{TurnSeq: 9}}, 0); !strings.Contains(got, "turn 9: delivered") {
		t.Fatalf("default verdict block = %q", got)
	}

	// The open-obligation count is the only thing the readiness ids contribute.
	open := renderTurnProgress(nil, 4)
	if !strings.Contains(open, "4 obligation(s) still open") {
		t.Fatalf("open-obligation block = %q", open)
	}
	if strings.Contains(open, "turn ") {
		t.Fatalf("no outcomes means no turn lines:\n%s", open)
	}
}

func TestRenderTurnProgressIsEmptyWithoutDataAndHonoursItsBudget(t *testing.T) {
	if got := renderTurnProgress(nil, 0); got != "" {
		t.Fatalf("an empty session renders nothing, got %q", got)
	}

	// The controller caps the window at three rounds, but the builder trims on its
	// own: a longer slice than the budget allows drops the oldest rounds first.
	outcomes := make([]agent.TurnOutcome, 0, 12)
	for turn := 1; turn <= 12; turn++ {
		outcomes = append(outcomes, agent.TurnOutcome{
			TurnSeq: turn, Verdict: agent.TurnVerdictBlocked, MissingCount: 9, ChangedFiles: 9, Recovered: 1,
		})
	}
	block := renderTurnProgress(outcomes, 5)
	if len([]rune(block)) > turnProgressBudget {
		t.Fatalf("block is %d runes, over the %d budget:\n%s", len([]rune(block)), turnProgressBudget, block)
	}
	if strings.Contains(block, "turn 1:") {
		t.Fatalf("over-budget blocks must drop the oldest round:\n%s", block)
	}
	if !strings.Contains(block, "turn 12:") {
		t.Fatalf("the newest round must survive:\n%s", block)
	}

	// A pathological single line is clipped rather than sent whole.
	long := renderTurnProgress([]agent.TurnOutcome{{TurnSeq: 1, Verdict: strings.Repeat("x", 400)}}, 0)
	if len([]rune(long)) > turnProgressBudget {
		t.Fatalf("a long verdict was not clipped: %d runes", len([]rune(long)))
	}
}
