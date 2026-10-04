package board

import (
	"strings"
	"testing"
	"time"
)

// The reclaim reason is the only place a reader learns why a claim ended: without the
// granted lease, "the holder died" and "the work outlived its own lease" read the same
// afterwards, and the deadline's second precision hides which of them it was (2026-10-05).
func TestReclaimReasonCarriesTheLeaseItEnded(t *testing.T) {
	claimed := time.Date(2026, 10, 5, 15, 0, 0, 123456789, time.UTC)
	n := &Node{
		ID:        "n1",
		State:     StateClaimed,
		Owner:     "alice",
		ClaimedAt: claimed,
		Deadline:  claimed.Add(30 * time.Minute),
	}

	op := ReclaimOp(n, claimed.Add(30*time.Minute+2*time.Second))
	for _, want := range []string{"alice", "15:30:00.123456789Z", "30m0s"} {
		if !strings.Contains(op.Reason, want) {
			t.Errorf("reclaim reason %q does not carry %q", op.Reason, want)
		}
	}
}

func TestReclaimReasonDoesNotInventALeaseForNodesThatNeverRecordedOne(t *testing.T) {
	n := &Node{
		ID:       "n1",
		State:    StateClaimed,
		Owner:    "alice",
		Deadline: time.Date(2026, 10, 5, 15, 30, 0, 0, time.UTC),
	}

	op := ReclaimOp(n, time.Date(2026, 10, 5, 15, 30, 2, 0, time.UTC))
	if strings.Contains(op.Reason, "lease") {
		t.Fatalf("reclaim reason claims a lease the node never recorded: %q", op.Reason)
	}
	if !strings.Contains(op.Reason, "alice") {
		t.Fatalf("reclaim reason lost the holder it ended: %q", op.Reason)
	}
}
