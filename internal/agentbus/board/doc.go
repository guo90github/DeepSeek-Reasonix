// Package board is the AGENT_BUS blackboard kernel: an append-only op log, a
// deterministic fold, the node state machine, and per-node leases.
//
// Its scope (docs/agents/AGENT_BUS.md §11.1) is this package alone — no
// transport, no session wiring, no scheduler; callers inject time and hand in a
// board directory. The log is the only truth: a write folds the log, validates
// the op against the folded state, then appends one line, all under a
// board-level file lock (internal/filelock) that serializes both goroutines in
// this process and other processes.
//
// Two rules keep replay honest. A trailing partial line is an uncommitted write
// and is never an op; a corrupt line is skipped but counted, never dropped
// silently.
package board
