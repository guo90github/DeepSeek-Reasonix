package agent

import (
	"fmt"

	"reasonix/internal/provider"
	"reasonix/internal/tool"
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
		return intervention{verdict: verdictAdvise, guidance: a.fanoutHint(a.turn.budget.rounds)}
	}
	return intervention{}
}

func batchingHint(streak int) string {
	return fmt.Sprintf("Host batching hint: the last %d rounds each sent a single read-only call, and every round is a full model round-trip. When the remaining work is independent, send those calls together in one message instead of one per round.", streak)
}

// fanoutHint names the fan-out options this run actually has. The second
// sentence appears only when parallel_calls is registered — that tool is what
// the plan_execute tier adds — so a run without the tier hears the same words it
// heard before, byte for byte.
func (a *Agent) fanoutHint(rounds int) string {
	hint := fmt.Sprintf("Host fan-out hint: this turn has taken %d rounds of read-only work. If the remaining work splits into independent areas, parallel_tasks runs them as concurrent sub-agents, so the wall clock is the slowest branch instead of the sum of rounds. Keep the calls in this message when they depend on each other, and prefer batching when the work is small; fan-out costs extra tokens because every branch has its own context.", rounds)
	if !a.hostCallsAvailable() {
		return hint
	}
	return hint + " When the nodes are concrete read-only calls you can already spell out, parallel_calls runs them inside this round instead: no sub-agent and no extra round trip."
}

// hostCallsAvailable reports whether this run registered the host-executed call
// surface. Asking the registry keeps one source of truth: the tier decides
// registration, and the hint follows it.
func (a *Agent) hostCallsAvailable() bool {
	if a == nil || a.svc.tools == nil {
		return false
	}
	_, ok := a.svc.tools.Get(tool.HostParallelCalls)
	return ok
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
