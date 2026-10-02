package control

import (
	"strings"

	"reasonix/internal/agent"
)

// budgetAxisToken is the axis name the agent's budget gate reports when the token limit
// is the one crossed (internal/agent/run_budget.go).
const (
	budgetAxisToken       = "token"
	stopCauseBudgetTokens = "budget_tokens"
)

func goalPauseFromRunError(err error) (cause, reason string, ok bool) {
	info, ok := agent.InspectRunPause(err)
	if !ok {
		return "", "", false
	}
	return goalPauseFromPause(info)
}

// goalPauseFromPause maps one run pause to the Goal stop cause it means. It takes the
// exported pause info rather than the error so the mapping can be tested directly —
// it is exactly where the axis used to be lost: every budget pause was reported as
// spend, so a token stop read as a spend stop and `budget_tokens` was written by nobody.
func goalPauseFromPause(info agent.RunPauseInfo) (cause, reason string, ok bool) {
	if !info.HostOwned || info.Kind != "task_budget" {
		return "", "", false
	}
	reason = strings.TrimSpace(info.Reason)
	if reason == "" {
		reason = "the Goal reached its spend budget"
	}
	if strings.TrimSpace(info.Key) == budgetAxisToken {
		return stopCauseBudgetTokens, reason, true
	}
	return stopCauseBudgetSpend, reason, true
}
