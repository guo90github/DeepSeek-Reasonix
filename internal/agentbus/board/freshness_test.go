package board

import (
	"strings"
	"testing"
	"time"
)

// The fold reads no clock (doc.go, fold.go: "It reads no clock"), and every op carries its own
// timestamps. So "this deadline is in the future" can only be a write-time guard: a claim that
// asks for a lease must carry a deadline, and it must be ahead of the writing host's clock —
// replay then applies what is in the log, lapse and all, because it has no clock to re-judge with.
func TestAFreshnessLapseIsJudgedAtWriteAndNeverAtReplay(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	undated := Op{Verb: VerbClaim, Node: "root", Actor: "alice"}
	if err := validateFreshness(undated, now); err == nil || !strings.Contains(err.Error(), ReasonMissingDeadline) {
		t.Fatalf("a claim without a deadline = %v, want %q", err, ReasonMissingDeadline)
	}
	lapsed := Op{Verb: VerbClaim, Node: "root", Actor: "alice", Deadline: now.Add(-time.Minute)}
	if err := validateFreshness(lapsed, now); err == nil || !strings.Contains(err.Error(), ReasonDeadlineNotFuture) {
		t.Fatalf("a claim whose deadline already passed = %v, want %q", err, ReasonDeadlineNotFuture)
	}
	live := Op{Verb: VerbClaim, Node: "root", Actor: "alice", Deadline: now.Add(time.Minute)}
	if err := validateFreshness(live, now); err != nil {
		t.Fatalf("a claim with a live deadline = %v, want it accepted", err)
	}
	// Not every verb is judged: an assertion carries no lease, so a stale deadline on one is not
	// the write-time guard's business.
	stale := opAssert("root", "alice")
	stale.Deadline = now.Add(-time.Hour)
	if err := validateFreshness(stale, now); err != nil {
		t.Fatalf("an assertion with an old deadline = %v, want it accepted (only leases are judged)", err)
	}

	// Replay: the write guard keeps a lapse out of the log, and what is in the log stays as
	// written (owner and deadline included). Explicit ids and seqs, because the fold counts a
	// reused or missing id as a rejection (TestFoldCountsDuplicateIDs pins that).
	state := Fold([]Op{
		{ID: "op-assert", Seq: 1, Verb: VerbAssert, Node: "root", Actor: "alice", Evidence: evidence("e1")},
		{ID: "op-claim", Seq: 2, Verb: VerbClaim, Node: "root", Actor: "alice", Deadline: lapsed.Deadline, Bounds: &Bounds{Steps: 1}},
	})
	if state.Applied != 2 {
		t.Fatalf("applied = %d, want replay to apply what the log holds", state.Applied)
	}
	if got := state.Nodes["root"]; got == nil || got.Owner != "alice" || !got.Deadline.Equal(lapsed.Deadline) {
		t.Fatalf("node = %+v, want the claim applied with the deadline the log carries", got)
	}
}
