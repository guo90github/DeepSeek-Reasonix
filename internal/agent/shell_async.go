package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
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

// promoteShellCallToBackground starts a shell call as a background job when the
// tier says the calls after it should not wait. The decision has to be made
// before launch: a foreground process cannot be handed over later, and killing
// and restarting it would repeat its side effects. Only a call with a later
// pending call in the same batch is promoted — with nothing left to continue
// with, waiting is the honest thing to do.
func (a *Agent) promoteShellCallToBackground(calls []provider.ToolCall, results []string, i int) bool {
	if a == nil || a.shellAsync == ShellAsyncOff || i+1 >= len(calls) {
		return false
	}
	call := &calls[i]
	if call.Name != "bash" && call.Name != "shell" {
		return false
	}
	var args map[string]any
	if json.Unmarshal([]byte(call.Arguments), &args) != nil {
		return false
	}
	if background, _ := args["run_in_background"].(bool); background {
		// The model asked for it itself, so it owns the ordering from here.
		return false
	}
	if a.shellAsync == ShellAsyncBalanced && !evidence.IsVerificationCommand(bashCommandFromArgs(json.RawMessage(call.Arguments))) {
		return false
	}
	later := false
	for j := i + 1; j < len(calls); j++ {
		if results[j] == "" {
			later = true
			break
		}
	}
	if !later {
		return false
	}
	args["run_in_background"] = true
	encoded, err := json.Marshal(args)
	if err != nil {
		return false
	}
	call.Arguments = string(encoded)
	return true
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
// batch so its trailing calls wait for the job it started.
func (a *Agent) promotedShellDefers(calls []provider.ToolCall, outcomes []toolOutcome, results []string, durations []int64, i int, promoted bool) bool {
	if !promoted || outcomes[i].errMsg != "" {
		return false
	}
	a.deferCallsAfterBackground(calls, outcomes, results, durations, i+1)
	return true
}
