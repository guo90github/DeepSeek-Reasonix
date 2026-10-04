package builtin

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// fallbackRejectHint is the exact sentence a caller reads when the board's reason names no
// correction. Pinned as a literal so a new reason cannot silently join it.
const fallbackRejectHint = "the board's reason is in the message above"

// boardReasons is the kernel's closed set of refusals (board/op.go). A refusal is the only
// place the board tells the caller what to do next, so every one of them has to name a
// correction: on the real machine the structural refusals all fell through to the fallback
// sentence, which names nothing (2026-10-05).
var boardReasons = []string{
	board.ReasonUnknownVerb,
	board.ReasonUnknownNode,
	board.ReasonIllegalTransition,
	board.ReasonMissingActor,
	board.ReasonMissingNode,
	board.ReasonMissingEvidence,
	board.ReasonMissingReason,
	board.ReasonMissingDeadline,
	board.ReasonMissingBounds,
	board.ReasonNotOwner,
	board.ReasonNotAssignee,
	board.ReasonMissingReproducer,
	board.ReasonSelfReproduced,
	board.ReasonMissingAbandonRequest,
	board.ReasonMissingDependency,
	board.ReasonDuplicateDependency,
	board.ReasonDuplicateNode,
	board.ReasonCycle,
	board.ReasonUnknownOutcome,
	board.ReasonInvalidOutcomeForState,
	board.ReasonDeadlineNotFuture,
	board.ReasonSystemOnly,
	board.ReasonDependencyClosed,
	board.ReasonIdempotencyConflict,
	board.ReasonRateLimited,
}

func TestEveryBoardReasonNamesACorrection(t *testing.T) {
	for _, reason := range boardReasons {
		hint := rejectHint(reason, board.VerbAssert)
		if strings.TrimSpace(hint) == "" {
			t.Errorf("reason %q has no hint", reason)
		}
		if hint == fallbackRejectHint {
			t.Errorf("reason %q falls through to the fallback hint", reason)
		}
	}
}

func TestAnUnknownReasonIsNotGuessedAt(t *testing.T) {
	if got := rejectHint("something_new", board.VerbAssert); got != fallbackRejectHint {
		t.Fatalf("an unknown reason got a hint that was invented: %q", got)
	}
}

// decide's missing_evidence is about the node's assertions, not about decide's own
// arguments: the old text sent the caller back to retry the same call (2026-10-05).
func TestDecideMissingEvidencePointsAtTheNodeNotTheCall(t *testing.T) {
	hint := rejectHint(board.ReasonMissingEvidence, board.VerbDecide)
	if !strings.Contains(hint, "assert") {
		t.Fatalf("decide's missing_evidence hint does not name the node-side assert: %q", hint)
	}
	if hint == rejectHint(board.ReasonMissingEvidence, board.VerbAssert) {
		t.Fatal("decide and assert share one hint; only decide's refusal is about the node")
	}
}
