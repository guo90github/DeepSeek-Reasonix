// Package jsonl stores newline-delimited JSON records that are only ever
// appended: one record per line, sync on write, torn tails repaired by the next
// writer, and corrupt lines counted rather than silently dropped. Both the
// blackboard op log and the talk log are built on it, so the two share one
// definition of what a damaged line means.
package jsonl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// DirPerm and FilePerm keep a log readable only by its owner.
	DirPerm  = 0o700
	FilePerm = 0o600
	// RepairWindow bounds how far back a torn tail is searched for its newline.
	RepairWindow = 64 * 1024
)

// Read is one pass over a log: the records that decoded, plus what was lost.
// The counters are the whole point — a reader that cannot say how much it
// skipped cannot be trusted to have replayed anything.
type Read[T any] struct {
	Items     []T
	Truncated int
	Skipped   int
}

// ReadAll decodes every complete line of path. A missing file is an empty log.
func ReadAll[T any](path string) (Read[T], error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Read[T]{}, nil
		}
		return Read[T]{}, fmt.Errorf("jsonl: read %s: %w", path, err)
	}
	return decode[T](data), nil
}

// ReadFrom decodes the complete lines that begin at offset, which is how a reader skips a prefix
// it has already folded. A log is append-only, so an offset taken at a line boundary stays valid
// as the file grows; proving that boundary belongs to the caller (see board's checkpoint). The
// line rules are ReadAll's: a partial trailing line and a corrupt line are counted, never parsed.
func ReadFrom[T any](path string, offset int64) (Read[T], error) {
	if offset < 0 {
		return Read[T]{}, fmt.Errorf("jsonl: negative offset for %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Read[T]{}, nil
		}
		return Read[T]{}, fmt.Errorf("jsonl: read %s: %w", path, err)
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return Read[T]{}, fmt.Errorf("jsonl: seek %s: %w", path, err)
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return Read[T]{}, fmt.Errorf("jsonl: read tail of %s: %w", path, err)
	}
	return decode[T](data), nil
}

// decode turns one buffer of lines into records, counting what it could not use: a trailing
// partial line is an uncommitted write, and a corrupt line is a damaged record. Neither is
// allowed to hide, and neither is allowed to break a read.
func decode[T any](data []byte) Read[T] {
	var out Read[T]
	if len(data) == 0 {
		return out
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
		var item T
		if err := json.Unmarshal(trimmed, &item); err != nil {
			out.Skipped++
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out
}

// Append writes one record as a single line and syncs it. A torn line can only
// ever be the last one, so a write first truncates it away: appending behind a
// partial line would bury the new record inside a corrupt one.
func Append(path string, v any) error {
	if _, err := RepairTornTail(path); err != nil {
		return err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("jsonl: encode: %w", err)
	}
	if strings.ContainsRune(string(raw), '\n') {
		return fmt.Errorf("jsonl: encoded record contains a newline")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, FilePerm)
	if err != nil {
		return fmt.Errorf("jsonl: open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("jsonl: append: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("jsonl: sync: %w", err)
	}
	return nil
}

// RepairTornTail drops an uncommitted trailing partial line and reports whether
// it changed the file. Callers run it inside their write lock.
func RepairTornTail(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, FilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("jsonl: open %s for repair: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("jsonl: stat %s: %w", path, err)
	}
	size := info.Size()
	if size == 0 {
		return false, nil
	}
	start := max(size-RepairWindow, 0)
	newSize := int64(-1)
	for {
		buf := make([]byte, size-start)
		if _, err := f.ReadAt(buf, start); err != nil {
			return false, fmt.Errorf("jsonl: read tail: %w", err)
		}
		if idx := bytes.LastIndexByte(buf, '\n'); idx >= 0 {
			newSize = start + int64(idx) + 1
			break
		}
		if start == 0 {
			newSize = 0
			break
		}
		start = max(start-RepairWindow, 0)
	}
	if newSize == size {
		return false, nil
	}
	if err := f.Truncate(newSize); err != nil {
		return false, fmt.Errorf("jsonl: truncate torn tail: %w", err)
	}
	if err := f.Sync(); err != nil {
		return false, fmt.Errorf("jsonl: sync repair: %w", err)
	}
	return true, nil
}

// EnsureDir creates dir with owner-only permissions and refuses a symlink, so a
// log can never be redirected somewhere else by a planted path.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("jsonl: mkdir: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("jsonl: stat dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("jsonl: refusing non-directory or symlink %s", dir)
	}
	return nil
}
