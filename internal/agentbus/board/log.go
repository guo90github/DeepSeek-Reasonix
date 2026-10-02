package board

import (
	"reasonix/internal/agentbus/jsonl"
)

const (
	logFileName  = "board.jsonl"
	lockFileName = "board.jsonl.lock"
	// filePerm and repairWindow mirror the shared log rules, so a test can plant a torn
	// tail exactly the way the next writer would find it.
	filePerm     = jsonl.FilePerm
	repairWindow = jsonl.RepairWindow
)

// logRead is one pass over the op log.
type logRead struct {
	Ops       []Op
	Truncated int
	Skipped   int
}

// readLog reads the op log under the shared jsonl rules: a trailing partial line is an
// uncommitted write and is counted, not parsed; a corrupt line is counted too. Neither is
// allowed to break a read. The mechanical part lives in jsonl so the board, the talk log,
// the hearing log and the queue all agree on what a damaged line means.
func readLog(path string) (logRead, error) {
	read, err := jsonl.ReadAll[Op](path)
	if err != nil {
		return logRead{}, err
	}
	return logRead{Ops: read.Items, Truncated: read.Truncated, Skipped: read.Skipped}, nil
}

func appendOp(path string, op Op) error { return jsonl.Append(path, op) }

// repairTornTail drops an uncommitted trailing partial line and reports whether it changed
// the file. It runs inside the write transaction's board lock.
func repairTornTail(path string) (bool, error) { return jsonl.RepairTornTail(path) }

func ensureDir(path string) error { return jsonl.EnsureDir(path) }
