package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// A steer the turn accepted but never applied must come back as a queued item.
// Acknowledging it deletes the guidance, which is how a remote wake was
// "delivered" (202) and then never executed: the room's @ stayed unanswered with
// nothing left to dispatch (measured 2026-09-23 against a real chatting room).
func TestInboxTurnDoneRequeuesUnappliedDurableSteer(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	rec, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentSteer, Submit: "room wake"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(rec.ItemID, sessioninbox.StateSteerAccepted, ""); err != nil {
		t.Fatal(err)
	}
	c.inbox.mu.Lock()
	c.inbox.clearActive()
	c.inbox.trackActive(rec.ItemID)
	c.inbox.mu.Unlock()
	// The turn is still completing when this runs, so admission stays busy: the
	// re-queue must be durable before the dispatcher claims it.
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.onInboxTurnDone()

	snap := c.InboxSnapshot()
	if len(snap.Items) != 1 {
		t.Fatalf("unapplied steer must survive turn completion, inbox=%+v", snap)
	}
	if got := snap.Items[0].State; got != sessioninbox.StateQueued {
		t.Fatalf("unapplied steer state = %q, want %q", got, sessioninbox.StateQueued)
	}
	if got := snap.Items[0].Intent; got != sessioninbox.IntentSteer {
		t.Fatalf("unapplied steer intent = %q, want %q", got, sessioninbox.IntentSteer)
	}
	if snap.Paused {
		t.Fatal("a re-queued wake must not park the inbox")
	}
}

// The applied case keeps its acknowledgement: a consumed steer is complete and
// must not come back as a duplicate turn.
func TestInboxTurnDoneStillAcksConsumedSteer(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	rec, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentSteer, Submit: "applied"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(rec.ItemID, sessioninbox.StateSteerConsumed, ""); err != nil {
		t.Fatal(err)
	}
	c.inbox.mu.Lock()
	c.inbox.clearActive()
	c.inbox.trackActive(rec.ItemID)
	c.inbox.mu.Unlock()

	c.onInboxTurnDone()

	if n := len(c.InboxSnapshot().Items); n != 0 {
		t.Fatalf("applied steer should be acknowledged away, still have %d items", n)
	}
}
