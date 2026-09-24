package agent

import (
	"os"
	"strings"
)

// The write barrier left by an interrupted call is off unless a value turns it
// back on. Only enforcement switches: receipts, statistics and the one-shot
// recovery handoff are still produced.
const envToolRecoveryBarrier = "REASONIX_TOOL_RECOVERY_BARRIER"

func toolRecoveryBarrierOff() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envToolRecoveryBarrier))) {
	case "on", "1", "true", "block", "enabled":
		return false
	}
	return true
}

// ToolRecoveryBarrierOff exposes the switch to controllers that read the
// durable pending set from this agent.
func ToolRecoveryBarrierOff() bool { return toolRecoveryBarrierOff() }
