package builtin

import (
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
	"reasonix/internal/evidence"
)

// writeLineSpans is the host's line accounting for a change: per hunk, the old
// lines it replaced and the newline delta it leaves for everything after them.
// A hunk landing mid-line invalidates the whole line it touches, and an
// insertion invalidates the line it lands on, which only ever costs a re-read.
func writeLineSpans(oldText, newText string) []evidence.WriteLineSpan {
	var spans []evidence.WriteLineSpan
	for _, edit := range udiff.Lines(oldText, newText) {
		first := strings.Count(oldText[:edit.Start], "\n") + 1
		last := strings.Count(oldText[:edit.End], "\n") + 1
		if edit.End > edit.Start && oldText[edit.End-1] == '\n' {
			last--
		}
		spans = append(spans, evidence.WriteLineSpan{
			FirstLine: first,
			LastLine:  last,
			Delta:     strings.Count(edit.New, "\n") - strings.Count(oldText[edit.Start:edit.End], "\n"),
		})
	}
	return spans
}
