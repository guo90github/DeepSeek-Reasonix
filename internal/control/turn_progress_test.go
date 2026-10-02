package control

import (
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

// BA2 (docs/70 §2.3): the pre-turn block states recent verdicts and counts only —
// never a prompt, a command, or a path.
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

// U-4 decided the block carries no character budget (docs/70 §五): the builder
// renders every round it is handed — the window is the controller's — and an
// undefined verdict collapses to one enum word instead of leaking sidecar text.
func TestRenderTurnProgressIsUncappedAndNormalisesTheVerdict(t *testing.T) {
	if got := renderTurnProgress(nil, 0); got != "" {
		t.Fatalf("an empty session renders nothing, got %q", got)
	}

	outcomes := make([]agent.TurnOutcome, 0, 12)
	for turn := 1; turn <= 12; turn++ {
		outcomes = append(outcomes, agent.TurnOutcome{
			TurnSeq: turn, Verdict: agent.TurnVerdictBlocked, MissingCount: 9, ChangedFiles: 9, Recovered: 1,
		})
	}
	block := renderTurnProgress(outcomes, 5)
	if !strings.Contains(block, "turn 1:") || !strings.Contains(block, "turn 12:") {
		t.Fatalf("the builder renders every round it is given:\n%s", block)
	}
	if len([]rune(block)) < 400 {
		t.Fatalf("block is only %d runes — a character budget crept back in:\n%s", len([]rune(block)), block)
	}

	odd := renderTurnProgress([]agent.TurnOutcome{{TurnSeq: 1, Verdict: strings.Repeat("x", 400)}}, 0)
	if !strings.Contains(odd, "turn 1: unknown") || strings.Contains(odd, "xxx") {
		t.Fatalf("an undefined verdict leaked into the block: %q", odd)
	}
	if len([]rune(odd)) > 120 {
		t.Fatalf("undefined verdict should render as one enum word, got %d runes: %q", len([]rune(odd)), odd)
	}
}

// The three-round window is the controller's own bound (U-4: at most 3 rounds).
func TestRecentTurnOutcomesKeepsTheNewestThreeRounds(t *testing.T) {
	dir := t.TempDir()
	path := agent.NewSessionPath(dir, "window")
	session := agent.NewSession("fixture system")
	exec := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	c := New(Options{Executor: exec, Sink: event.Discard, SessionDir: dir})
	defer c.Close()
	c.SetFreshSessionPath(path)

	for turn := 1; turn <= 5; turn++ {
		outcome := agent.TurnOutcome{TurnSeq: turn, Verdict: agent.TurnVerdictDelivered}
		if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
			agent.AppendTurnOutcome(meta, outcome)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	got := c.recentTurnOutcomes()
	if len(got) != turnProgressRounds {
		t.Fatalf("window = %d rounds, want %d: %+v", len(got), turnProgressRounds, got)
	}
	if got[0].TurnSeq != 3 || got[len(got)-1].TurnSeq != 5 {
		t.Fatalf("window should hold the newest rounds oldest-first, got %+v", got)
	}
}
