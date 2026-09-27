package control

import (
	"testing"

	"reasonix/internal/sessioninbox"
)

// The room's contract names `paused` as the field it folds into "held". A receipt
// that says paused=false while the queue holding that item is paused is worse
// than saying nothing: it is the field a sender trusts, and it is the one the
// answered wake has to agree with `gate`.
func TestFollowupReceiptTellsTheTruthAboutAPausedQueue(t *testing.T) {
	c, _, _ := newInboxDispatchController(t)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	rec, err := c.TryEnqueueFollowup(InboxRequest{Submit: "wake", Idempotency: "paused-1"})
	if err != nil {
		t.Fatal(err)
	}

	if rec.Gate != sessioninbox.GatePaused {
		t.Fatalf("gate = %q, want %q", rec.Gate, sessioninbox.GatePaused)
	}
	if !rec.Paused {
		t.Fatal("paused = false while the queue holding this item is paused")
	}
	if rec.Resumable == nil || *rec.Resumable {
		t.Fatalf("resumable = %v, want an explicit false: only a human resumes a paused queue", rec.Resumable)
	}
	if rec.GateReason == "" {
		t.Fatal("a paused receipt with no sentence leaves the sender nothing to relay")
	}
}
