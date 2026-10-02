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
//
// No character budget: U-4 decided none (docs/70 §五), and the block cannot run
// away — the controller caps the window at turnProgressRounds and every line is
// an enum word plus small counters (see turnProgressVerdict).
const turnProgressRounds = 3

// renderTurnProgress is the whole block builder: outcomes oldest first, then the
// open-obligation count. The window is the controller's; an empty input renders
// nothing at all.
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
	return wrapTurnProgress(lines)
}

// renderTurnOutcomeLine states one turn's verdict and the counters it carries.
func renderTurnOutcomeLine(outcome agent.TurnOutcome) string {
	line := fmt.Sprintf("- turn %d: %s", outcome.TurnSeq, turnProgressVerdict(outcome.Verdict))
	if count := outcome.MissingCount; count > 0 {
		line += fmt.Sprintf(", %d open obligation(s)", count)
	}
	if count := outcome.ChangedFiles; count > 0 {
		line += fmt.Sprintf(", %d file(s) changed", count)
	}
	if outcome.Recovered > 0 {
		line += ", recovered"
	}
	return line
}

// turnProgressVerdict normalises the stored verdict to the kit's enum. Empty is
// the delivered default (AppendTurnOutcome writes it that way); a value the kit
// does not define renders as unknown instead of leaking whatever the sidecar
// held — which is what bounds the block without a character budget.
func turnProgressVerdict(verdict string) string {
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "", agent.TurnVerdictDelivered:
		return agent.TurnVerdictDelivered
	case agent.TurnVerdictBlocked:
		return agent.TurnVerdictBlocked
	case agent.TurnVerdictAborted:
		return agent.TurnVerdictAborted
	default:
		return "unknown"
	}
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
