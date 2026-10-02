package agentbus

import (
	"testing"

	"reasonix/internal/agentbus/board"
)

func weightEvidence(kind, ref string) board.Evidence {
	return board.Evidence{Kind: kind, Ref: ref}
}

func TestVerifiableWeightCountsEachReferenceOnce(t *testing.T) {
	cases := []struct {
		name     string
		evidence []board.Evidence
		want     int
	}{
		{"a bare claim weighs nothing", []board.Evidence{weightEvidence("opinion", "")}, 0},
		{"whitespace is not a reference", []board.Evidence{weightEvidence("opinion", "   ")}, 0},
		{"two references weigh two", []board.Evidence{weightEvidence("test", "a"), weightEvidence("diff", "b")}, 2},
		{"the same reference twice is one", []board.Evidence{weightEvidence("test", "a"), weightEvidence("test", "a")}, 1},
		{"trimmed duplicates are one too", []board.Evidence{weightEvidence("test", " a "), weightEvidence("diff", "a")}, 1},
		{"mixed claims and references", []board.Evidence{
			weightEvidence("opinion", ""), weightEvidence("test", "a"), weightEvidence("opinion", "a"),
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifiableWeight(tc.evidence); got != tc.want {
				t.Fatalf("weight = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRepeatingOneReferenceDoesNotOutweighIt is the verdict-level half: repetition must
// not turn a tie into a refutation. One verifiable reference on each side is still a
// tie, so the question goes to a human rather than being decided by who cited more
// (T6-6, T5-6).
func TestRepeatingOneReferenceDoesNotOutweighIt(t *testing.T) {
	claim := []board.Evidence{weightEvidence("test", "go test ./internal/agentbus/...")}
	repeated := []board.Evidence{
		weightEvidence("test", "same-ref"), weightEvidence("test", "same-ref"), weightEvidence("diff", "same-ref"),
	}
	verdict, reason := WeighResponse(claim, repeated, 0, 1)
	if verdict != VerdictEscalate || reason != ReasonEqualWeight {
		t.Fatalf("repeated citation of one reference = (%q, %q), want it to stay a tie", verdict, reason)
	}

	// Two genuinely different references do outweigh one, so the rule is not "nothing
	// counts twice": it is "each reference counts once".
	distinct := []board.Evidence{weightEvidence("test", "one"), weightEvidence("diff", "two")}
	verdict, reason = WeighResponse(claim, distinct, 0, 1)
	if verdict != VerdictRefuted || reason != ReasonWeight {
		t.Fatalf("two distinct references = (%q, %q), want the refutation to stand", verdict, reason)
	}
}
