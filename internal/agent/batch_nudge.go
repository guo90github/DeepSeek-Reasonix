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

// applyBatchNudge asks the model to fold independent read-only calls into one
// message once a run of single-call rounds makes the waste visible. It fires at
// most once per turn, and only for a run of productive, unblocked read-only
// rounds — a stuck round belongs to the loop guards, and batching a loop would
// only multiply its wasted calls.
func (a *Agent) applyBatchNudge(calls []provider.ToolCall, outcomes []toolOutcome, receiptMark int) intervention {
	if a == nil {
		return intervention{}
	}
	if len(calls) != 1 || !batchOutcomesAreReadOnly(outcomes) || !batchOutcomesSucceeded(outcomes) || !a.batchRoundMadeProgress(receiptMark) {
		a.turn.loop.resetBatchingStreak()
		return intervention{}
	}
	streak := a.turn.loop.bumpBatchingStreak()
	if streak < batchNudgeStreak || !a.turn.loop.markBatchingNudged() {
		return intervention{}
	}
	return intervention{
		verdict:  verdictAdvise,
		guidance: fmt.Sprintf("Host batching hint: the last %d rounds each sent a single read-only call, and every round is a full model round-trip. When the remaining work is independent, send those calls together in one message instead of one per round.", streak),
	}
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
