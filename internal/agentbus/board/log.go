package board

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	logFileName  = "board.jsonl"
	lockFileName = "board.jsonl.lock"
	dirPerm      = 0o700
	filePerm     = 0o600
	repairWindow = 64 * 1024
)

// logRead is one pass over the op log.
type logRead struct {
	Ops       []Op
	Truncated int
	Skipped   int
}

// readLog reads the op log line by line. A trailing partial line is an
// uncommitted write and is counted, not parsed; a corrupt line is counted too.
// Neither is allowed to break a read.
func readLog(path string) (logRead, error) {
	var out logRead
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("board: read log: %w", err)
	}
	if len(data) == 0 {
		return out, nil
	}
	lines := bytes.Split(data, []byte("\n"))
	if tail := lines[len(lines)-1]; len(tail) > 0 {
		out.Truncated++
	}
	lines = lines[:len(lines)-1]
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			out.Skipped++
			continue
		}
		var op Op
		if err := json.Unmarshal(trimmed, &op); err != nil {
			out.Skipped++
			continue
		}
		out.Ops = append(out.Ops, op)
	}
	return out, nil
}

// appendOp writes one op as a single JSON line and syncs it. A torn line can
// only ever be the last one, so a write first truncates it away: appending
// behind a partial line would bury the new record inside a corrupt line.
func appendOp(path string, op Op) error {
	if _, err := repairTornTail(path); err != nil {
		return err
	}
	raw, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("board: encode op: %w", err)
	}
	if strings.ContainsRune(string(raw), '\n') {
		return fmt.Errorf("board: encoded op contains a newline")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("board: open log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("board: append op: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("board: sync log: %w", err)
	}
	return nil
}

// repairTornTail drops an uncommitted trailing partial line and reports whether
// it changed the file. It runs inside the write transaction's board lock.
func repairTornTail(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, filePerm)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("board: open log for repair: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("board: stat log: %w", err)
	}
	size := info.Size()
	if size == 0 {
		return false, nil
	}
	start := max(0, size-repairWindow)
	newSize := int64(-1)
	for {
		buf := make([]byte, size-start)
		if _, err := f.ReadAt(buf, start); err != nil {
			return false, fmt.Errorf("board: read log tail: %w", err)
		}
		if idx := bytes.LastIndexByte(buf, '\n'); idx >= 0 {
			newSize = start + int64(idx) + 1
			break
		}
		if start == 0 {
			newSize = 0
			break
		}
		if start -= repairWindow; start < 0 {
			start = 0
		}
	}
	if newSize == size {
		return false, nil
	}
	if err := f.Truncate(newSize); err != nil {
		return false, fmt.Errorf("board: truncate torn tail: %w", err)
	}
	if err := f.Sync(); err != nil {
		return false, fmt.Errorf("board: sync repair: %w", err)
	}
	return true, nil
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, dirPerm); err != nil {
		return fmt.Errorf("board: mkdir: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("board: stat dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("board: refusing non-directory or symlink %s", path)
	}
	return nil
}
