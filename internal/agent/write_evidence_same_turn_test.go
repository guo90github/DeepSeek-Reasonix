package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// roundOutcome runs one provider round through the real batch pipeline so a
// test can read the diagnostic a blocked call produced, not just its text.
func roundOutcome(t *testing.T, a *Agent, id, name string, args any) (string, toolOutcome) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	a.freezeVisibleReads(a.Session().Snapshot())
	calls := []provider.ToolCall{{ID: id, Name: name, Arguments: string(raw)}}
	a.sess.conversation.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: calls})
	b := a.executeBatch(context.Background(), &a.turn, calls)
	if len(b.results) != 1 || len(b.outcomes) != 1 {
		t.Fatalf("round %s produced results=%v outcomes=%d", id, b.results, len(b.outcomes))
	}
	return b.results[0], b.outcomes[0]
}

// readWindow reads one window through the real reader. limit 0 reads the
// reader's default whole-file window.
func readWindow(t *testing.T, read tool.Tool, path string, offset, limit int) tool.ReadWindow {
	t.Helper()
	args := map[string]any{"path": path}
	if limit > 0 {
		args["offset"], args["limit"] = offset, limit
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := read.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("read_file fixture: %v", err)
	}
	window, ok := tool.ParseReadWindow(out)
	if !ok {
		t.Fatalf("read_file produced no window: %q", out)
	}
	return window
}

func blockedCode(out toolOutcome) string {
	if out.diagnostic == nil {
		return ""
	}
	return out.diagnostic.Code
}

func latestObservation(t *testing.T, a *Agent, path string) evidence.TextObservation {
	t.Helper()
	var latest evidence.TextObservation
	found := false
	for _, o := range a.task.ledger.TextObservations() {
		if o.Path != path {
			continue
		}
		if !found || o.Sequence > latest.Sequence {
			latest, found = o, true
		}
	}
	if !found {
		t.Fatalf("no observation was filed for %s", path)
	}
	return latest
}

func gateOn(t *testing.T, declared tool.EvidenceTargetInfo, filed evidence.TextObservation) (*Agent, string) {
	t.Helper()
	a, ledger := newEvidenceAgent(t, evidenceWriter{target: declared}, true)
	ledger.RecordTextObservation(filed)
	out, blocked := runEvidenceGate(a, declared.Path)
	if !blocked {
		return a, ""
	}
	return a, out.output
}

// The SESSION-65 repro: one turn, one file, two edits. The write retires the
// lines it replaced, so the second edit of the line just written owes the model
// a read round it already paid for.
func TestSameTurnSecondEditOfARewrittenLineNeedsNoReadRound(t *testing.T) {
	path := writeFixture(t, "alpha\nbeta\ngamma\n")
	a := lifecycleAgent(t)

	roundOutcome(t, a, "r1", "read_file", map[string]any{"path": path})
	if first, _ := roundOutcome(t, a, "e1", "edit_file", map[string]any{"path": path, "old_string": "beta", "new_string": "delta"}); strings.Contains(first, "blocked:") {
		t.Fatalf("fixture: the first edit must be accepted: %q", first)
	}

	second, outcome := roundOutcome(t, a, "e2", "edit_file", map[string]any{"path": path, "old_string": "delta", "new_string": "epsilon"})
	if code := blockedCode(outcome); code == tool.WriteEvidenceStale || code == tool.WriteEvidenceMissing {
		t.Fatalf("the second edit was blocked as %s: %q", code, second)
	}
	if strings.Contains(second, "evidence required") {
		t.Fatalf("the second edit was blocked: %q", second)
	}
	if got := fixtureBody(t, path); !strings.Contains(got, "epsilon") {
		t.Fatalf("file = %q, want the second edit applied", got)
	}
}

// Evidence after a write must be what the read pipeline reports for the lines
// the write produced, never the bytes on disk: a CRLF file is where they differ,
// because the reader normalizes the carriage return the write arguments omit.
func TestPostWriteEvidenceComesFromTheReadPipelineNotTheWriteArguments(t *testing.T) {
	path := writeFixture(t, "alpha\r\nbeta\r\ngamma\r\n")
	a := lifecycleAgent(t)
	read, ok := tool.LookupBuiltin("read_file")
	if !ok {
		t.Fatal("read_file builtin is not registered")
	}

	roundOutcome(t, a, "r1", "read_file", map[string]any{"path": path})
	if first, _ := roundOutcome(t, a, "e1", "edit_file", map[string]any{"path": path, "old_string": "beta", "new_string": "delta"}); strings.Contains(first, "blocked:") {
		t.Fatalf("fixture: the first edit must be accepted: %q", first)
	}

	// The line the write produced, as the read pipeline reports it.
	window := readWindow(t, read, path, 1, 1)
	fromPipeline := make([]string, len(window.Lines))
	for i, line := range window.Lines {
		fromPipeline[i] = hashLine(line)
	}

	filed := latestObservation(t, a, path)
	if !slices.Equal(filed.LineHashes, fromPipeline) || filed.StartLine != window.StartLine {
		t.Fatalf("filed evidence is not what the read pipeline reports: filed=%v start=%d want=%v start=%d",
			filed.LineHashes, filed.StartLine, fromPipeline, window.StartLine)
	}
	// Binding this window to a source identity would make it usable only while
	// reader and writer agree on that identity; host-read content is current by
	// construction, so it carries none.
	if filed.Snapshot != "" {
		t.Fatalf("the host-read window must not be bound to a source identity: %q", filed.Snapshot)
	}

	// The bytes on disk for that line still carry the CRLF the reader strips, so
	// a host that hashed the file itself would have filed something else.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rawLine2 := strings.Split(string(raw), "\n")[1]
	if hashLine(rawLine2) == filed.LineHashes[0] {
		t.Fatal("fixture: the disk line must differ from the reader's view, or this test proves nothing")
	}

	// The gate approves exactly the produced line ...
	declared := tool.EvidenceTargetInfo{
		Path:   filed.Path,
		Ranges: []tool.ReadRange{{Start: window.StartLine - 1, End: window.StartLine - 1 + len(fromPipeline)}},
		Hashes: fromPipeline,
	}
	if _, blocked := gateOn(t, declared, evidence.TextObservation{Path: filed.Path, StartLine: window.StartLine, LineHashes: fromPipeline}); blocked != "" {
		t.Fatalf("the line the write produced must satisfy the gate: %s", blocked)
	}

	// ... and only it: the rest of the file was not written here, so the model
	// still owes a read before overwriting it.
	whole := readWindow(t, read, path, 0, 0)
	allHashes := make([]string, len(whole.Lines))
	for i, line := range whole.Lines {
		allHashes[i] = hashLine(line)
	}
	overwrite := tool.EvidenceTargetInfo{
		Path:      filed.Path,
		WholeFile: true,
		Ranges:    []tool.ReadRange{{Start: 0, End: len(allHashes)}},
		Hashes:    allHashes,
	}
	if _, blocked := gateOn(t, overwrite, evidence.TextObservation{Path: filed.Path, StartLine: window.StartLine, LineHashes: fromPipeline}); blocked == "" {
		t.Fatal("a whole-file overwrite must still owe the rest of the file")
	}
}

// The third SESSION-65 class: a partial read, then an edit outside the window. The
// refusal names the missing lines but does not hand them over, so the retry costs a
// read round. Landing that handover is A-4, prototyped and reverted (docs/10 §十).
func TestPartialReadBlockCarriesTheLinesItAsksFor(t *testing.T) {
	t.Skip("A-4 (deliver the missing lines in the refusal) is prototyped in the root-cause map, not landed")

	path := writeFixture(t, "one\ntwo\nthree\nfour\nfive\n")
	a := lifecycleAgent(t)
	roundOutcome(t, a, "r1", "read_file", map[string]any{"path": path, "offset": 0, "limit": 2})

	out, outcome := roundOutcome(t, a, "e1", "edit_file", map[string]any{"path": path, "old_string": "four", "new_string": "FOUR"})
	if code := blockedCode(outcome); code != tool.WriteEvidenceStale {
		t.Fatalf("an edit outside the read window must be refused as stale: %q (%s)", out, code)
	}
	if !strings.Contains(out, "4-4") {
		t.Fatalf("the refusal must name the lines it wants: %q", out)
	}
	if !strings.Contains(out, "four") {
		t.Fatalf("the refusal must carry the lines it asks for, or the retry costs a read round: %q", out)
	}
	if retry, _ := roundOutcome(t, a, "e2", "edit_file", map[string]any{"path": path, "old_string": "four", "new_string": "FOUR"}); strings.Contains(retry, "blocked:") {
		t.Fatalf("the retry after a delivered refusal must be accepted: %q", retry)
	}
	if got := fixtureBody(t, path); !strings.Contains(got, "FOUR") {
		t.Fatalf("file = %q, want the retry applied", got)
	}
}

// A whole-file write is authored end to end, so the lines it leaves behind are
// the model's own text and the next edit of that file owes no read round.
func TestWholeFileWriteAlsoFilesWhatItAuthored(t *testing.T) {
	path := writeFixture(t, "alpha\nbeta\ngamma\n")
	editor, ok := tool.LookupBuiltin("edit_file")
	if !ok {
		t.Fatal("edit_file builtin is not registered")
	}
	overwrite, ok := tool.LookupBuiltin("write_file")
	if !ok {
		t.Fatal("write_file builtin is not registered")
	}
	a := newIncompleteReadTestAgent(&scriptedProvider{}, incompleteReadBuiltin(t), NewSession("sys"), event.Discard, editor, overwrite)
	a.task.ledger = evidence.NewLedger()

	roundOutcome(t, a, "r1", "read_file", map[string]any{"path": path})
	if out, _ := roundOutcome(t, a, "w1", "write_file", map[string]any{"path": path, "content": "alpha\nBETA\ngamma\n"}); strings.Contains(out, "blocked:") || strings.Contains(out, "error:") {
		t.Fatalf("fixture: the overwrite must be accepted: %q", out)
	}
	out, outcome := roundOutcome(t, a, "e1", "edit_file", map[string]any{"path": path, "old_string": "BETA", "new_string": "delta"})
	if code := blockedCode(outcome); code == tool.WriteEvidenceStale || code == tool.WriteEvidenceMissing {
		t.Fatalf("editing the line a whole-file write produced was blocked as %s: %q", code, out)
	}
	if got := fixtureBody(t, path); !strings.Contains(got, "delta") {
		t.Fatalf("file = %q, want the edit applied", got)
	}
}

// An insertion is authored end to end too: every line it puts in the file is
// the model's own text, so the file the write left behind is fully evidence.
func TestInsertionFilesEveryLineItProduced(t *testing.T) {
	for _, tc := range []struct {
		name, before, old, replacement string
	}{
		{
			name:        "one line grows into a block",
			before:      "alpha\nbeta\ngamma\ndelta\n",
			old:         "gamma",
			replacement: "one\ntwo\nthree\ngamma",
		},
		{
			name:        "a changed header and a block inserted after it",
			before:      "// first header\n// second header\nfunc f() {\n}\n\ntail\n",
			old:         "// first header\n// second header\nfunc f() {",
			replacement: "// new header\n// second header\nfunc a() {\n}\n\nfunc b() {\n\treturn\n}\n\nfunc f() {",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFixture(t, tc.before)
			a := lifecycleAgent(t)
			roundOutcome(t, a, "r1", "read_file", map[string]any{"path": path})

			edit := map[string]any{"path": path, "old_string": tc.old, "new_string": tc.replacement}
			if out, _ := roundOutcome(t, a, "e1", "edit_file", edit); strings.Contains(out, "blocked:") || strings.Contains(out, "error:") {
				t.Fatalf("fixture: the edit must be accepted: %q", out)
			}

			whole := readWindow(t, incompleteReadBuiltin(t), path, 0, 0)
			filed := map[int]string{}
			for _, o := range a.task.ledger.TextObservations() {
				if o.Path != path {
					continue
				}
				for i, hash := range o.LineHashes {
					filed[o.StartLine+i] = hash
				}
			}
			var holes []string
			for i, line := range whole.Lines {
				if got := filed[whole.StartLine+i]; got != hashLine(line) {
					holes = append(holes, fmt.Sprintf("%d %q", whole.StartLine+i, line))
				}
			}
			if len(holes) > 0 {
				t.Fatalf("lines the write produced are not evidence: %s", strings.Join(holes, ", "))
			}
		})
	}
}

// The measured shape of the same defect: iterating on one region of a file in
// a single turn. Every edit after the first owes a read round until the host
// files what the write produced.
func TestSameTurnRepeatedEditsOfOneLineNeedNoRereads(t *testing.T) {
	path := writeFixture(t, "alpha\nbeta\ngamma\n")
	a := lifecycleAgent(t)
	roundOutcome(t, a, "r1", "read_file", map[string]any{"path": path})

	previous := "beta"
	blocked := 0
	for i := range 5 {
		next := fmt.Sprintf("v%d", i)
		out, outcome := roundOutcome(t, a, fmt.Sprintf("e%d", i), "edit_file", map[string]any{"path": path, "old_string": previous, "new_string": next})
		if code := blockedCode(outcome); code == tool.WriteEvidenceStale || code == tool.WriteEvidenceMissing {
			blocked++
			// What the model has to do about it: read the file again.
			roundOutcome(t, a, fmt.Sprintf("r%d", i), "read_file", map[string]any{"path": path})
			continue
		}
		if strings.Contains(out, "blocked:") {
			t.Fatalf("edit %d was blocked by something else: %q", i, out)
		}
		previous = next
	}
	if blocked != 0 {
		t.Fatalf("a one-region turn spent %d rounds on evidence rejections", blocked)
	}
	if got := fixtureBody(t, path); !strings.Contains(got, "v4") {
		t.Fatalf("file = %q, want the last edit applied", got)
	}
}
