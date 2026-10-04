package control

import "reasonix/internal/agentbus"

// dispatchWakeKey names the wake one dispatch carries. A verification is a different act from a
// fresh run, so it gets its own key: the wake ledger collapses repeats by key, and a verification
// swallowed as "already told about this node" is exactly the waste the intent removes
// (F15, 2026-10-05).
func dispatchWakeKey(board, node string, verify bool) string {
	if verify {
		return agentbus.VerifyKey(board, node)
	}
	return agentbus.DispatchKey(board, node)
}
