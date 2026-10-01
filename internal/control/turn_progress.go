package control

import (
	"fmt"
	"strings"

	"reasonix/internal/agent"
)

// Pre-turn progress block (docs/70 §2.3): the session's recent turn outcomes and
// the obligations the readiness gate left open, so a later turn starts knowing
// where it stands. It rides the turn body like the memory-update and recall
// blocks — never the cache-stable prefix — and carries verdicts and counts only.
//
// The kernel's obligation "ids" are descriptive sentences (they name commands),
// so they do not enter this block: the model already receives them through the
// final-readiness channel at the moment the gate blocks an answer.
const (
	turnProgressRounds  = 3
	turnProgressBudget  = 400
	turnProgressLineCap = 160
)

// renderTurnProgress is the whole block builder: outcomes oldest first, then the
// open-obligation count. Older rounds are dropped first when the budget is tight,
// and an empty input renders nothing at all.
func renderTurnProgress(outcomes []agent.TurnOutcome, openObligations int) string {
	lines := make([]string, 0, len(outcomes)+1)
	for _, outcome := range outcomes {
		lines = append(lines, renderTurnOutcomeLine(outcome))
	}
	if openObligations > 0 {
		lines = append(lines, fmt.Sprintf("- %d obligation(s) still open from the last readiness check", openObligations))
	}
	if len(lines) == 0 {
		return ""
	}
	for len(lines) > 1 {
		block := wrapTurnProgress(lines)
		if len([]rune(block)) <= turnProgressBudget {
			return block
		}
		lines = lines[min(1, len(lines)-1):]
	}
	return wrapTurnProgress(lines)
}

// renderTurnOutcomeLine states one turn's verdict and the counters it carries.
func renderTurnOutcomeLine(outcome agent.TurnOutcome) string {
	verdict := strings.TrimSpace(outcome.Verdict)
	if verdict == "" {
		verdict = agent.TurnVerdictDelivered
	}
	line := fmt.Sprintf("- turn %d: %s", outcome.TurnSeq, verdict)
	if count := outcome.MissingCount; count > 0 {
		line += fmt.Sprintf(", %d open obligation(s)", count)
	}
	if count := outcome.ChangedFiles; count > 0 {
		line += fmt.Sprintf(", %d file(s) changed", count)
	}
	if outcome.Recovered > 0 {
		line += ", recovered"
	}
	return clipTurnProgressLine(line)
}

func clipTurnProgressLine(line string) string {
	runes := []rune(line)
	if len(runes) <= turnProgressLineCap {
		return line
	}
	return string(runes[:turnProgressLineCap-1]) + "…"
}

func wrapTurnProgress(lines []string) string {
	return strings.Join(append([]string{"<turn-progress>"}, append(lines, "</turn-progress>")...), "\n")
}

// turnProgressBlock reads the record and renders the block. Empty when the session
// has nothing to report, so a fresh session sends nothing extra.
func (c *Controller) turnProgressBlock() string {
	if c == nil {
		return ""
	}
	outcomes := c.recentTurnOutcomes()
	open := 0
	if c.executor != nil {
		open = len(c.executor.LastMissingObligations())
	}
	return renderTurnProgress(outcomes, open)
}

// recentTurnOutcomes returns the newest outcomes, oldest first, capped at the
// block's round window.
func (c *Controller) recentTurnOutcomes() []agent.TurnOutcome {
	path := strings.TrimSpace(c.SessionPath())
	if path == "" {
		return nil
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		return nil
	}
	outcomes := make([]agent.TurnOutcome, 0, len(meta.TurnOutcome))
	for _, outcome := range meta.TurnOutcome {
		if outcome.TurnSeq < 1 {
			// A record without a turn number has nothing to place in the window.
			continue
		}
		outcomes = append(outcomes, outcome)
	}
	if len(outcomes) > turnProgressRounds {
		outcomes = outcomes[len(outcomes)-turnProgressRounds:]
	}
	return append([]agent.TurnOutcome(nil), outcomes...)
}
