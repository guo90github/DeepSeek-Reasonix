package builtin

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// abandon kept being refused because the caller passed a reason where the board wants a ref —
// four times in a row on a real session: the hint has to say that a reason is not evidence
// (2026-10-05).
func TestAbandonMissingEvidenceSaysAReasonIsNotEvidence(t *testing.T) {
	hint := rejectHint(board.ReasonMissingEvidence, board.VerbAbandon)
	if !strings.Contains(hint, "reason") || !strings.Contains(hint, "evidence") {
		t.Fatalf("abandon's missing_evidence hint does not separate a reason from evidence: %q", hint)
	}
	if hint == rejectHint(board.ReasonMissingEvidence, board.VerbAssert) {
		t.Fatal("abandon and assert share one hint; abandon's refusal is about its own ref")
	}
}
