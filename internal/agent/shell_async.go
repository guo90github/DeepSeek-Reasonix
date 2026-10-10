package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/shellsafe"
	"reasonix/internal/tool"
)

// ShellAsyncTier is how aggressively a shell call is started as a background
// job so the loop does not wait for it. The zero value keeps today's behavior.
type ShellAsyncTier int

const (
	// ShellAsyncOff runs every call in the foreground.
	ShellAsyncOff ShellAsyncTier = iota
	// ShellAsyncBalanced backgrounds the calls the host recognizes as checks
	// and builds — the long ones a batch rarely depends on immediately.
	ShellAsyncBalanced
	// ShellAsyncFast backgrounds any shell call that shares its batch with a
	// later call, which is the fastest tier and the least predictable.
	ShellAsyncFast
)

// ParseShellAsyncTier reads the configured tier. An unknown value stays off:
// guessing here would change when commands run without being asked to.
func ParseShellAsyncTier(value string) ShellAsyncTier {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fast", "极速":
		return ShellAsyncFast
	case "balanced", "平衡":
		return ShellAsyncBalanced
	default:
		return ShellAsyncOff
	}
}

func (t ShellAsyncTier) String() string {
	switch t {
	case ShellAsyncFast:
		return "fast"
	case ShellAsyncBalanced:
		return "balanced"
	default:
		return "off"
	}
}

// promoteShellCallToBackground decides whether a shell call starts off the
// critical path. The verdict travels out of band (tool.WithOffPathLaunch) and
// never edits the call the model wrote — the descriptor reports the launch
// instead. The decision has to be made before launch: a foreground process
// cannot be handed over later.
func (a *Agent) promoteShellCallToBackground(calls []provider.ToolCall, results []string, i int) bool {
	if a == nil || i < 0 || i >= len(calls) || !offPathShellTool(calls[i].Name) {
		return false
	}
	var args map[string]any
	if json.Unmarshal([]byte(calls[i].Arguments), &args) != nil {
		return false
	}
	background, _ := args["run_in_background"].(bool)
	return a.shellAsync.DecideOffPath(OffPathRequest{
		Kind:          OffPathShellCall,
		Tool:          calls[i].Name,
		HostKnowsLong: evidence.IsVerificationCommand(bashCommandFromArgs(json.RawMessage(calls[i].Arguments))),
		ModelAsked:    background,
		Later:         a.offPathLater(results, i+1, len(calls)),
	}).OffPath
}

// offPathLater is what the policy reads as work to continue with: a pending
// call in this batch, or planned work the turn still owes. A finished checklist
// leaves nothing to overlap, so a lone call keeps its place.
func (a *Agent) offPathLater(results []string, start, end int) LaterWork {
	if later := offPathLaterWork(results, start, end); later != LaterWorkNone {
		return later
	}
	if _, incomplete := a.canonicalTodoProgress(); incomplete {
		return LaterWorkTurnPending
	}
	return LaterWorkNone
}

// deferCallsAfterBackground fills the calls a promotion left behind. They must
// not run before the job finishes: an early read can observe a half-applied
// change, and an early check can report on a state nobody asked about.
func (a *Agent) deferCallsAfterBackground(calls []provider.ToolCall, outcomes []toolOutcome, results []string, durations []int64, start int) {
	for j := start; j < len(calls); j++ {
		if results[j] != "" {
			continue
		}
		msg := fmt.Sprintf("blocked: not run — an earlier call in this batch was moved to the background and is still running. "+
			"Collect it first (bash_output with its job id, or wait), then send %s again.", calls[j].Name)
		var ex *tool.ShellExecution
		if calls[j].Name == "bash" || calls[j].Name == "shell" {
			ex = &tool.ShellExecution{
				Kind:         "shell",
				State:        tool.ShellStateNotRun,
				FailurePhase: tool.ShellPhaseDependency,
				MutationRisk: tool.ShellMutationNotStarted,
			}
		}
		results[j] = msg
		outcomes[j] = toolOutcome{output: msg, blocked: true, errMsg: firstLine(msg), execution: ex}
		durations[j] = 0
	}
}

// settlePromotedBatch finalizes a batch whose shell call went to the background:
// the calls behind it wait for that job instead of racing it.
func (a *Agent) settlePromotedBatch(calls []provider.ToolCall, outcomes []toolOutcome, results []string, durations []int64, start, end int, finalize func(int)) {
	for j := start; j < end; j++ {
		if results[j] == "" {
			a.deferCallsAfterBackground(calls, outcomes, results, durations, j)
		}
		finalize(j)
	}
}

// promotedShellDefers reports whether a promoted shell call must suspend this
// batch so its trailing calls wait for the job it started. A command the host
// can prove leaves durable state alone (and unknown commands never qualify as
// proven) cannot make a trailing call observe a half-applied change, so only
// the ones that might mutate keep their batch behind them.
func (a *Agent) promotedShellDefers(calls []provider.ToolCall, outcomes []toolOutcome, results []string, durations []int64, i int, promoted bool) bool {
	if !promoted || outcomes[i].errMsg != "" {
		return false
	}
	if !shellsafe.ClassifyBash(bashCommandFromArgs(json.RawMessage(calls[i].Arguments))).AnyMutation() {
		return false
	}
	a.deferCallsAfterBackground(calls, outcomes, results, durations, i+1)
	return true
}
