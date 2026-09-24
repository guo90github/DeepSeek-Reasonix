package builtin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"reasonix/internal/tool"
)

var staleAnchorQuotedLine = regexp.MustCompile("(?m)^[ \t]*\\d+\u2192.*$")

// quotedStaleAnchorWindow extracts the numbered lines a stale-anchor failure
// quoted back, as the model would copy them.
func quotedStaleAnchorWindow(t *testing.T, msg string) string {
	t.Helper()
	lines := staleAnchorQuotedLine.FindAllString(msg, -1)
	if len(lines) == 0 {
		t.Fatalf("error carries no quoted current text: %q", msg)
	}
	return strings.Join(lines, "\n") + "\n"
}

// quotedStaleAnchorNumbers returns the line numbers of the quoted window.
func quotedStaleAnchorNumbers(t *testing.T, report string) []int {
	t.Helper()
	var numbers []int
	for _, line := range staleAnchorQuotedLine.FindAllString(strings.TrimRight(report, "\n"), -1) {
		number, _, _ := strings.Cut(strings.TrimSpace(line), "\u2192")
		value, err := strconv.Atoi(number)
		if err != nil {
			t.Fatalf("quoted line %q carries no line number", line)
		}
		numbers = append(numbers, value)
	}
	if len(numbers) == 0 {
		t.Fatalf("report quotes no current text: %q", report)
	}
	return numbers
}

func TestStaleAnchorReportQuotesCurrentText(t *testing.T) {
	content := "package main\n\nfunc main() {\n\told := 1\n\tfmt.Println(old)\n}\n\nfunc other() {}\n"
	anchor := "func main() {\n\tnewer := 1\n\tfmt.Println(newer)\n}"

	report := staleAnchorReport(anchor, content)
	for _, want := range []string{
		"nearest current text is lines 3-6",
		"action: " + tool.RecoveryRereadTarget,
		"read_file offset=2 limit=4",
		"   3\u2192func main() {",
		"   4\u2192\told := 1",
		"   6\u2192}",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report %q does not contain %q", report, want)
		}
	}
	if strings.Contains(report, "newer") {
		t.Fatalf("report quotes the stale anchor instead of the current text: %q", report)
	}
}

func TestStaleAnchorReportIgnoresUnrelatedAnchor(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	for _, anchor := range []string{
		"nothing here resembles the file",
		"completely different content\nand another line",
	} {
		if report := staleAnchorReport(anchor, content); report != "" {
			t.Fatalf("report for unrelated anchor %q = %q, want empty", anchor, report)
		}
	}
}

func TestStaleAnchorReportStaysBounded(t *testing.T) {
	var b strings.Builder
	for i := range 400 {
		if i == 200 {
			b.WriteString("\ttarget := aVeryLongLineOfSourceThatKeepsGoingAndGoingAndGoingForTwoHundredBytesAtLeast()\n")
			continue
		}
		b.WriteString("\tpadding := 0\n")
	}
	content := b.String()
	anchor := "\ttarget := aVeryLongLineOfSourceThatKeepsGoingAndGoingAndGoingForTwoHundredBytesAtLeast()\n\tmissing := 1"

	report := staleAnchorReport(anchor, content)
	if report == "" {
		t.Fatal("expected a quoted window for a drifting anchor")
	}
	quoted := strings.Split(strings.TrimRight(quotedStaleAnchorWindow(t, report), "\n"), "\n")
	if len(quoted) > staleAnchorWindowLines {
		t.Fatalf("quoted %d lines, want at most %d", len(quoted), staleAnchorWindowLines)
	}
	if len(report) > staleAnchorWindowBytes+512 {
		t.Fatalf("report is %d bytes, want it bounded around %d", len(report), staleAnchorWindowBytes)
	}
	if !strings.Contains(report, "lines 201-202") {
		t.Fatalf("report lost the anchored line: %q", report)
	}
}

func TestStaleAnchorReportClipsLongLines(t *testing.T) {
	long := strings.Repeat("x", 900)
	content := "head\n" + long + "\ntail\n"
	anchor := strings.Repeat("x", 900) + "drift"

	report := staleAnchorReport(anchor, content)
	if report == "" {
		t.Fatal("expected a quoted window for a long-line anchor")
	}
	for _, line := range staleAnchorQuotedLine.FindAllString(report, -1) {
		if len(line) > staleAnchorLineBytes+32 {
			t.Fatalf("quoted line is %d bytes, want it clipped to %d: %q", len(line), staleAnchorLineBytes, line)
		}
	}
	if !strings.Contains(report, "\u2026[truncated]") {
		t.Fatalf("clipped line is not marked as clipped: %q", report)
	}
}

func TestStaleAnchorWindowKeepsTheMatchingLine(t *testing.T) {
	var content, anchor strings.Builder
	for i := range 60 {
		fmt.Fprintf(&content, "\tline%02d := 0\n", i)
	}
	for i := range 30 {
		fmt.Fprintf(&anchor, "\tline%02d := 1\n", i)
	}

	report := staleAnchorReport(strings.TrimSuffix(anchor.String(), "\n"), content.String())
	if report == "" {
		t.Fatal("expected a quoted window for a long drifting anchor")
	}
	numbers := quotedStaleAnchorNumbers(t, report)
	if len(numbers) > staleAnchorWindowLines {
		t.Fatalf("quoted %d lines, want at most %d", len(numbers), staleAnchorWindowLines)
	}
	if numbers[0] != 1 {
		t.Fatalf("window starts at line %d, want the first mapped anchor line 1: %q", numbers[0], report)
	}
}

// A file whose last line has no newline still gets a window, as long as the
// window itself stops before that line.
func TestStaleAnchorReportQuotesBeforeAFileWithNoTrailingNewline(t *testing.T) {
	content := "alpha old\nbeta old\ngamma\nomega-tail"
	anchor := "alpha new\nbeta new"

	report := staleAnchorReport(anchor, content)
	for _, want := range []string{"nearest current text is lines 1-2", "   2\u2192beta old\n"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report %q does not contain %q", report, want)
		}
	}
}

func TestEditFileStaleAnchorWindowIsRetryable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	seed := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\told := 1\n\tfmt.Println(old)\n}\n\nfunc other() {}\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": "func main() {\n\tnewer := 1\n\tfmt.Println(newer)\n}",
		"new_string": "func main() {\n\tnewer := 1\n\tfmt.Println(newer)\n}",
	}))
	if err == nil {
		t.Fatal("expected the stale anchor to be rejected")
	}
	var opErr *tool.OperationError
	if !errors.As(err, &opErr) || opErr.Diagnostic.Code != tool.WriteEvidenceStale {
		t.Fatalf("error %v carries diagnostic %+v, want %s", err, opErr, tool.WriteEvidenceStale)
	}
	if !strings.Contains(err.Error(), "old_string not found") {
		t.Fatalf("error %q lost its original cause", err.Error())
	}
	if want := "nearest current text is lines 5-8; action: " + tool.RecoveryRereadTarget +
		"; re-read that range (read_file offset=4 limit=4, offset is 0-based), or copy the current text below and retry:"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not carry %q", err.Error(), want)
	}

	window := quotedStaleAnchorWindow(t, err.Error())
	out, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": window,
		"new_string": "func main() {\n\told := 2\n\tfmt.Println(old)\n}",
	}))
	if err != nil {
		t.Fatalf("copying the quoted current text did not fix the edit: %v\nquoted window:\n%s", err, window)
	}
	if !strings.Contains(out, "fuzzy match") {
		t.Fatalf("retry output = %q, want the read_file-prefixed copy to match", out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "old := 2") || strings.Contains(string(got), "old := 1") {
		t.Fatalf("content = %q, want the retry applied", got)
	}
}

// A CRLF file is the common case on Windows: the window quotes LF-only lines,
// so the retry only works if the existing line-ending normalization picks it up.
func TestEditFileStaleAnchorWindowRoundTripsCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "NOTES.md")
	seed := "alpha\r\nbeta\r\ngamma old\r\ndelta\r\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": "gamma new\ndelta",
		"new_string": "gamma new\ndelta",
	}))
	if err == nil {
		t.Fatal("expected the stale anchor to be rejected")
	}
	if !strings.Contains(err.Error(), "CRLF line endings") {
		t.Fatalf("error %q does not name the line-ending hazard", err.Error())
	}

	window := quotedStaleAnchorWindow(t, err.Error())
	// The quoted window ends each line with its newline, so the replacement has
	// to cover the same span — the shape the model's own anchor would have had.
	if _, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": window,
		"new_string": "gamma new\ndelta\n",
	})); err != nil {
		t.Fatalf("copying the quoted window did not fix the CRLF edit: %v\nquoted window:\n%s", err, window)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "alpha\r\nbeta\r\ngamma new\r\ndelta\r\n" {
		t.Fatalf("content = %q, want the CRLF retry applied in place", got)
	}
}

// A window ending at a last line without a newline cannot be quoted back
// faithfully, so that failure keeps the prose answer instead of a copy that
// would gain a newline the file does not have.
func TestEditFileStaleAnchorStaysProseAtAFileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tail.txt")
	seed := "alpha\nbeta\ngamma\nomega-tail"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": "gamma\ndelta-tail",
		"new_string": "gamma\ndelta-tail",
	}))
	if err == nil {
		t.Fatal("expected the stale anchor to be rejected")
	}
	if strings.Contains(err.Error(), "nearest current text") {
		t.Fatalf("error %q quotes a window it cannot render faithfully", err.Error())
	}
	if !strings.Contains(err.Error(), "old_string not found") {
		t.Fatalf("error %q lost its original cause", err.Error())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != seed {
		t.Fatalf("content = %q, want the file untouched", got)
	}
}

// An anchor that resembles nothing in the file keeps today's prose answer: an
// arbitrary quote would invite an edit against a region the caller never meant.
func TestEditFileInjectedAnchorKeepsProseOnlyMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PROBE.md")
	if err := os.WriteFile(path, []byte("alpha\r\nbeta\r\ngamma\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": "zzz-probe-never-matches-zzz-37",
		"new_string": "probe-no-match",
	}))
	if err == nil {
		t.Fatal("expected the injected anchor to be rejected")
	}
	for _, gone := range []string{"nearest current text", "action: " + tool.RecoveryRereadTarget} {
		if strings.Contains(err.Error(), gone) {
			t.Fatalf("error %q quotes a window for an invented anchor", err.Error())
		}
	}
	if !strings.Contains(err.Error(), "old_string not found") {
		t.Fatalf("error %q lost its original cause", err.Error())
	}
}
