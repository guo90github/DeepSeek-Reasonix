package agent

import (
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// commitWrite mirrors the production order: the write's receipt lands in the
// ledger first, then the rebase re-anchors the windows that predate it.
func commitWrite(a *Agent, spans []evidence.WriteLineSpan) {
	plan := &toolCallPlan{
		call:                provider.ToolCall{Name: "edit_file", Arguments: `{"path":"/w/a.go"}`},
		expectedWriteSource: tool.EvidenceTargetInfo{Path: "/w/a.go", LineSpans: spans},
	}
	rec := a.task.ledger.Record(evidence.Receipt{ToolName: "edit_file", Success: true, Write: true, Paths: []string{"/w/a.go"}})
	a.rebaseWriteObservations(plan, rec, nil)
}

func TestRebaseKeepsEvidenceForTheLinesAWriteLeftAlone(t *testing.T) {
	writer := evidenceWriter{target: tool.EvidenceTargetInfo{
		Path: "/w/a.go", Ranges: []tool.ReadRange{{Start: 3, End: 4}}, Hashes: hashesFor("d"),
	}}
	a, ledger := newEvidenceAgent(t, writer, true)
	ledger.RecordTextObservation(evidence.TextObservation{
		Path: "/w/a.go", StartLine: 1, Snapshot: "ss2:1", LineHashes: hashesFor("a", "b", "c", "d", "e"),
	})

	commitWrite(a, []evidence.WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: 0}})

	if out, blocked := runEvidenceGate(a, "/w/a.go"); blocked {
		t.Fatalf("a line the write left alone must keep its evidence: %+v", out)
	}
}

func TestRebaseStillBlocksTheLineTheWriteReplaced(t *testing.T) {
	writer := evidenceWriter{target: tool.EvidenceTargetInfo{
		Path: "/w/a.go", Ranges: []tool.ReadRange{{Start: 1, End: 2}}, Hashes: hashesFor("B"),
	}}
	a, ledger := newEvidenceAgent(t, writer, true)
	ledger.RecordTextObservation(evidence.TextObservation{
		Path: "/w/a.go", StartLine: 1, Snapshot: "ss2:1", LineHashes: hashesFor("a", "b", "c", "d", "e"),
	})

	commitWrite(a, []evidence.WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: 0}})

	if out, blocked := runEvidenceGate(a, "/w/a.go"); !blocked {
		t.Fatalf("the line the write replaced is not evidence: %+v", out)
	}
}

func TestRebaseFollowsTheLinesAnInsertionMoved(t *testing.T) {
	writer := evidenceWriter{target: tool.EvidenceTargetInfo{
		Path: "/w/a.go", Ranges: []tool.ReadRange{{Start: 6, End: 7}}, Hashes: hashesFor("e"),
	}}
	a, ledger := newEvidenceAgent(t, writer, true)
	ledger.RecordTextObservation(evidence.TextObservation{
		Path: "/w/a.go", StartLine: 1, Snapshot: "ss2:1", LineHashes: hashesFor("a", "b", "c", "d", "e"),
	})

	commitWrite(a, []evidence.WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: 2}})

	if out, blocked := runEvidenceGate(a, "/w/a.go"); blocked {
		t.Fatalf("two inserted lines must move the window with the file: %+v", out)
	}
}

func TestWriteWithoutLineAccountingStillRetiresEvidence(t *testing.T) {
	writer := evidenceWriter{target: tool.EvidenceTargetInfo{
		Path: "/w/a.go", Ranges: []tool.ReadRange{{Start: 3, End: 4}}, Hashes: hashesFor("d"),
	}}
	a, ledger := newEvidenceAgent(t, writer, true)
	ledger.RecordTextObservation(evidence.TextObservation{
		Path: "/w/a.go", StartLine: 1, Snapshot: "ss2:1", LineHashes: hashesFor("a", "b", "c", "d", "e"),
	})

	commitWrite(a, nil)

	if out, blocked := runEvidenceGate(a, "/w/a.go"); !blocked {
		t.Fatalf("a writer that cannot account for its change retires the windows: %+v", out)
	}
}
