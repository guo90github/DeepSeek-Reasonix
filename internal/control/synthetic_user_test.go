package control

import (
	"testing"

	"reasonix/internal/agent"
)

func TestIsSyntheticUserMessageFinishProtocolRepair(t *testing.T) {
	for _, input := range []string{
		"Protocol repair: finish this turn now. A visible final answer has already been provided, so do not repeat it. Call finish exactly once as the only tool call with outcome completed, partial, or blocked.",
		"Protocol repair: finish this turn now. The finish call has already been accepted, so do not call it again. Provide the visible final answer now.",
		"Protocol repair: finish this turn now. Provide the visible final answer and call finish exactly once as the only tool call. If you need the user's answer instead, call ask and do not call finish.",
	} {
		if !IsSyntheticUserMessage(input) {
			t.Fatalf("finish protocol repair %q must be classified synthetic", input)
		}
	}
}

func TestIsSyntheticUserMessageRepairWrappedInTransientBlocks(t *testing.T) {
	// withTurnPreferences wraps the injected repair prompt in transient
	// preference blocks; the persisted shape must still be synthetic.
	input := agent.WithReasoningLanguage(
		agent.WithResponseLanguage("Protocol repair: finish this turn now. A visible final answer has already been provided, so do not repeat it.", "zh"),
		"zh")
	if !IsSyntheticUserMessage(input) {
		t.Fatalf("wrapped finish protocol repair %q must be classified synthetic", input)
	}
}

func TestIsSyntheticUserMessageHostNudges(t *testing.T) {
	// Openings of every remaining user-role nudge the agent loop persists
	// (finalization, run_loop, goal_run_boundary). Keep in sync with the
	// injection sites named in legacySyntheticUserPrefixes.
	for _, input := range []string{
		"The following tools are unavailable in the current workflow phase: bash, task. Do not call them again. Respond to the user's request with visible answer text now; call a different tool only if it is still needed to complete the request.",
		"Auto recovery has reached its limit for this turn. Do not call any more tools. Summarize what was completed, what failed, and what the user should do next. The user can continue in the next message.",
		"Host progress check: the current todo has produced no new completion, unique read, command, or mutation for 12 tool-call rounds. Reassess before using more tools: sign off the current item if it is done, narrow the remaining work without replacing the active item, or explain/ask about a real blocker. Do not repeat reads, commands, or writes just to reset this guard.",
		"Host progress redirect: the current todo still has no new completion or unique host-observed work after 16 tool-call rounds. Re-plan and continue: shrink the active step, switch tools or approach, delegate a focused sub-task, or use update_goal(blocked) only if a user or external condition is the sole blocker. Do not repeat the same calls.",
		"This task has reached its spend budget. Do not call any more tools. Synthesize a final answer from the work already completed: what was accomplished, what remains, and any decision the user should make. Use the evidence already collected and label what is still uncertain; the user can continue in the next message.",
		"Your tool-call round limit (max_steps) has been reached. Do not call any more tools. Synthesize a final answer from the work already completed: what was accomplished, what remains, and any decision the user should make. The user can increase max_steps or continue in the next turn if more work is needed.",
	} {
		if !IsSyntheticUserMessage(input) {
			t.Fatalf("host nudge %q must be classified synthetic", input)
		}
	}
}
