package agent

// OffPathKind names the work that asks to start off the critical path. A shell
// call whose batch has later work is the only caller today; batch nodes,
// speculative rounds and ingest distillation join by adding a kind here so one
// switch governs what leaves the critical path.
type OffPathKind int

const (
	OffPathShellCall OffPathKind = iota
)

// LaterWork is what still waits behind a call, which is what the fast tier
// reads as "there is work to continue with".
type LaterWork int

const (
	LaterWorkNone LaterWork = iota
	LaterWorkSameBatch
	// LaterWorkTurnPending is work the turn still owes even when the batch has
	// nothing left in it: an unfinished canonical todo.
	LaterWorkTurnPending
)

// OffPathRequest is every input the decision reads, so the rule is testable
// without a live batch.
type OffPathRequest struct {
	Kind          OffPathKind
	Tool          string
	HostKnowsLong bool
	ModelAsked    bool
	Later         LaterWork
}

// OffPathDecision carries the answer and why, so a tier that declines is never
// silent about it.
type OffPathDecision struct {
	OffPath bool
	Reason  string
}

// DecideOffPath is the single place any work asks to leave the critical path.
// Tiers keep their configured meanings: off never lifts, balanced lifts only
// host-recognized long work, fast lifts anything that has later work.
func (t ShellAsyncTier) DecideOffPath(req OffPathRequest) OffPathDecision {
	if req.Kind == OffPathShellCall && !offPathShellTool(req.Tool) {
		return OffPathDecision{Reason: "not a shell call"}
	}
	if req.ModelAsked {
		return OffPathDecision{Reason: "the model asked for the background and owns the ordering that follows"}
	}
	if req.Later == LaterWorkNone {
		return OffPathDecision{Reason: "nothing later to continue with; waiting is the honest thing to do"}
	}
	switch t {
	case ShellAsyncOff:
		return OffPathDecision{Reason: "tier off: every call runs in the foreground"}
	case ShellAsyncBalanced:
		if !req.HostKnowsLong {
			return OffPathDecision{Reason: "balanced lifts only host-recognized checks and builds"}
		}
		return OffPathDecision{OffPath: true, Reason: "balanced: a host-recognized check has later work to overlap with"}
	case ShellAsyncFast:
		return OffPathDecision{OffPath: true, Reason: "fast: later work should not wait"}
	default:
		return OffPathDecision{Reason: "unknown tier"}
	}
}

// offPathShellTool is the set the shell tier covers, kept in one place: the
// caller short-circuits on it and the policy gates on it.
func offPathShellTool(name string) bool { return name == "bash" || name == "shell" }

// offPathLaterWork reports whether a call at or after start is still pending,
// which is the work promotion needs something to overlap with.
func offPathLaterWork(results []string, start, end int) LaterWork {
	for j := start; j < end; j++ {
		if results[j] == "" {
			return LaterWorkSameBatch
		}
	}
	return LaterWorkNone
}
