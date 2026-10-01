package evidence

import (
	"path/filepath"
	"strings"
)

// WriteLineSpan is the host's line accounting for one hunk of an applied
// write: the 1-based inclusive old lines it replaced, and the newlines its
// replacement added (negative when it removed some). Everything after
// LastLine moves by Delta; the span's own lines stop being evidence.
type WriteLineSpan struct {
	FirstLine int
	LastLine  int
	Delta     int
}

func (s WriteLineSpan) invalidates(line int) bool {
	return line >= s.FirstLine && line <= s.LastLine
}

func shiftBefore(spans []WriteLineSpan, line int) int {
	shift := 0
	for _, s := range spans {
		if s.LastLine < line {
			shift += s.Delta
		}
	}
	return shift
}

// RebaseTextObservation translates one model-visible window into the version a
// write left behind: replaced lines are dropped and everything after them
// moves by the recorded delta, so later edits rely only on lines the model
// really saw.
//
// The result is line-hash evidence for the new version and nothing more. It
// binds no snapshot and answers no old read token, so both are cleared.
func RebaseTextObservation(o TextObservation, spans []WriteLineSpan) []TextObservation {
	if len(spans) == 0 || len(o.LineHashes) == 0 {
		return []TextObservation{o}
	}
	var out []TextObservation
	var run TextObservation
	flush := func() {
		if len(run.LineHashes) > 0 {
			out = append(out, run)
			run = TextObservation{}
		}
	}
	for i, hash := range o.LineHashes {
		line := o.StartLine + i
		if anyInvalidates(spans, line) {
			flush()
			continue
		}
		if len(run.LineHashes) == 0 {
			run.Path = o.Path
			run.StartLine = line + shiftBefore(spans, line)
		}
		run.LineHashes = append(run.LineHashes, hash)
	}
	flush()
	return out
}

func anyInvalidates(spans []WriteLineSpan, line int) bool {
	for _, s := range spans {
		if s.invalidates(line) {
			return true
		}
	}
	return false
}

// WrittenLines names the lines a write authored, 1-based and inclusive, in the
// coordinates of the file it produced. That is exactly the content the model
// can be said to know afterwards: it composed those lines itself. The rest of
// the file was not written here, so sight of it is unchanged.
func WrittenLines(spans []WriteLineSpan) [][2]int {
	var out [][2]int
	for _, s := range spans {
		count := s.LastLine - s.FirstLine + 1 + s.Delta
		if count <= 0 {
			continue
		}
		start := s.FirstLine + shiftBefore(spans, s.FirstLine)
		out = append(out, [2]int{start, start + count - 1})
	}
	return out
}

// RebaseObservations re-anchors path's model-visible windows through one
// applied write. The surviving windows are stamped after it, so the write no
// longer hides them and a later edit still needs no second read.
//
// Absent observations carry no window and are left untouched.
func (l *Ledger) RebaseObservations(path string, spans []WriteLineSpan) {
	if len(spans) == 0 {
		return
	}
	l.reanchorObservations(path, spans, false)
}

// RetireObservations drops path's model-visible windows. A write whose line
// accounting the host cannot express moves the file into coordinates no window
// describes; keeping them would let a later rebase shift an older version's
// lines as if they described the current one.
func (l *Ledger) RetireObservations(path string) {
	l.reanchorObservations(path, nil, true)
}

func (l *Ledger) reanchorObservations(path string, spans []WriteLineSpan, retire bool) {
	if l == nil {
		return
	}
	canonical := filepath.Clean(strings.TrimSpace(path))
	if canonical == "" || canonical == "." {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]TextObservation, 0, len(l.observations))
	for _, o := range l.observations {
		if o.Absent || filepath.Clean(strings.TrimSpace(o.Path)) != canonical {
			out = append(out, o)
			continue
		}
		if retire {
			continue
		}
		for _, piece := range RebaseTextObservation(o, spans) {
			l.nextSequence++
			piece.Sequence = l.nextSequence
			out = append(out, piece)
		}
	}
	l.observations = out
}
