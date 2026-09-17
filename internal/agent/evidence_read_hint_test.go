package agent

import (
	"strings"
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

// A write whose lines are byte-identical to a read the model was already shown
// can cite that read's handle instead of spending a round re-reading the file.
func TestEvidenceGateOffersTheSeenReadHandle(t *testing.T) {
	writer := evidenceWriter{target: tool.EvidenceTargetInfo{
		Path: "/w/a.go", WholeFile: true, Hashes: hashesFor("alpha", "beta"),
	}}
	a, ledger := newEvidenceAgent(t, writer, true)
	read := ledger.Record(evidence.Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{"/w/a.go"}})
	ledger.RecordTextObservation(evidence.TextObservation{
		Path: "/w/a.go", StartLine: 1, Token: read.ID, LineHashes: hashesFor("alpha", "beta"),
	})
	// The write that followed retired the read's window for the automatic path.
	ledger.Record(evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"/w/a.go"}})

	out, blocked := runEvidenceGate(a, "/w/a.go")
	if !blocked {
		t.Fatalf("a retired read must still be blocked on the automatic path: %+v", out)
	}
	if !strings.Contains(out.output, read.ID) {
		t.Fatalf("the block should name the read handle the model already saw: %q", out.output)
	}
}

// Without a read the model was shown there is nothing to cite, so the block
// stays a plain re-read instruction.
func TestEvidenceGateOffersNoHandleWithoutARead(t *testing.T) {
	writer := evidenceWriter{target: tool.EvidenceTargetInfo{
		Path: "/w/a.go", WholeFile: true, Hashes: hashesFor("alpha", "beta"),
	}}
	a, _ := newEvidenceAgent(t, writer, true)

	out, blocked := runEvidenceGate(a, "/w/a.go")
	if !blocked {
		t.Fatalf("an overwrite without evidence must be blocked: %+v", out)
	}
	if strings.Contains(out.output, "source_token") {
		t.Fatalf("no read handle exists, so none may be offered: %q", out.output)
	}
}
