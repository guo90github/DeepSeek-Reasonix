package agent

import (
	"fmt"

	"reasonix/internal/provider"
)

// batchNudgeStreak is how many consecutive rounds may carry a single read-only
// call before the host points out that independent reads belong in one message.
// Rounds, not calls, are what make a turn slow, so one short tail line is worth
// it — the hint rides the round tail and never touches the cached prefix.
const batchNudgeStreak = 3

// fanoutNudgeMinRounds is the round count at which a read-only turn that
// already batched its calls is long enough that its work may split into
// independent areas for sub-agents.
const fanoutNudgeMinRounds = 8

// fanoutIgnoredFixRounds is how many further single-call rounds after the
// batching hint count as the model ignoring it — the same patience that hint
// showed, so the escalation keeps the same rhythm.
const fanoutIgnoredFixRounds = batchNudgeStreak

// applyBatchNudge points at the cheaper fix first: fold independent read-only
// calls into one message (or recognize that the model already batches), and only
// then, on a turn that still runs long, offer the fan-out alternative that
// overlaps work at a token cost. Each hint fires at most once per turn, and only
// while the round was productive, unblocked and read-only.
func (a *Agent) applyBatchNudge(calls []provider.ToolCall, outcomes []toolOutcome, receiptMark int) intervention {
	if a == nil {
		return intervention{}
	}
	a.turn.budget.parallelism.observe(calls)
	if !batchOutcomesAreReadOnly(outcomes) || !batchOutcomesSucceeded(outcomes) || !a.batchRoundMadeProgress(receiptMark) {
		a.turn.loop.resetBatchingStreak()
		return intervention{}
	}
	if len(calls) != 1 {
		a.turn.loop.markBatchingInUse()
		a.turn.loop.resetBatchingStreak()
	} else if streak := a.turn.loop.bumpBatchingStreak(); streak >= batchNudgeStreak && a.turn.loop.markBatchingNudged() {
		return intervention{verdict: verdictAdvise, guidance: batchingHint(streak)}
	}
	if a.turn.loop.fanoutWorthOffering(a.turn.budget.rounds) && a.turn.loop.markFanoutNudged() {
		return intervention{verdict: verdictAdvise, guidance: fanoutHint(a.turn.budget.rounds)}
	}
	return intervention{}
}

func batchingHint(streak int) string {
	return fmt.Sprintf("Host batching hint: the last %d rounds each sent a single read-only call, and every round is a full model round-trip. When the remaining work is independent, send those calls together in one message instead of one per round.", streak)
}

func fanoutHint(rounds int) string {
	return fmt.Sprintf("Host fan-out hint: this turn has taken %d rounds of read-only work. If the remaining work splits into independent areas, parallel_tasks runs them as concurrent sub-agents, so the wall clock is the slowest branch instead of the sum of rounds. Keep the calls in this message when they depend on each other, and prefer batching when the work is small; fan-out costs extra tokens because every branch has its own context.", rounds)
}

// batchRoundMadeProgress reports whether this round earned at least one
// successful receipt. A ledger-less run keeps the outcome check as its proof.
func (a *Agent) batchRoundMadeProgress(receiptMark int) bool {
	if a.task.ledger == nil {
		return true
	}
	for _, rec := range a.task.ledger.ReceiptsSince(receiptMark) {
		if rec.Success {
			return true
		}
	}
	return false
}

// batchOutcomesAreReadOnly reports whether a round left durable state alone,
// judged the way the batch executor does: no workspace mutation, and no resolved
// call that turned out to be a writer.
func batchOutcomesAreReadOnly(outcomes []toolOutcome) bool {
	for _, outcome := range outcomes {
		if outcome.workspaceMutation != nil || (outcome.resolved && !outcome.resolvedReadOnly) {
			return false
		}
	}
	return true
}

func batchOutcomesSucceeded(outcomes []toolOutcome) bool {
	for _, outcome := range outcomes {
		if outcome.blocked || outcome.errMsg != "" {
			return false
		}
	}
	return true
}
