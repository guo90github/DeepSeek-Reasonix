package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// A steer the turn never took must name the gate that closed: "no turn to
// steer" and "the turn refused the steer" need different answers.
func TestRejectedSteerNamesTheClosedGate(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(session, []byte("{}\n"), 0o644)
	runner := &gatedTurnRunner{started: make(chan struct{}), release: make(chan struct{})}
	c := New(Options{Runner: runner, SessionPath: session, SessionDir: dir, Sink: event.Discard})
	defer c.autosaveWG.Wait()
	defer close(runner.release)

	rec, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentSteer, Submit: "mid-turn please"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.TrySteerInboxItem(rec.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposition != sessioninbox.DispositionQueuedFollowup {
		t.Fatalf("disposition = %s, want queued_followup", got.Disposition)
	}
	if got.SteerRejected != sessioninbox.SteerRejectedNoRunningTurn {
		t.Fatalf("steerRejected = %q, want %q", got.SteerRejected, sessioninbox.SteerRejectedNoRunningTurn)
	}
	// Nothing holds the downgraded item at the instant it is reported; the
	// dispatcher starts it as its own turn right after this receipt.
	if got.DispatchGate != "" {
		t.Fatalf("dispatchGate = %q, want empty for a steer nothing is blocking", got.DispatchGate)
	}

	<-runner.started
	waitForRunning(t, c)
	staleRec, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentSteer, Submit: "stale"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = c.TrySteerInboxItemForTurn("inbox-item-steer-for-an-ended-turn", staleRec.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SteerRejected != sessioninbox.SteerRejectedStaleTurn {
		t.Fatalf("steerRejected = %q, want %q", got.SteerRejected, sessioninbox.SteerRejectedStaleTurn)
	}
	if got.DispatchGate != sessioninbox.DispatchGateTurnRunning {
		t.Fatalf("dispatchGate = %q, want %q", got.DispatchGate, sessioninbox.DispatchGateTurnRunning)
	}
}

func TestPausedSteerNamesThePauseGate(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(session, []byte("{}\n"), 0o644)
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	defer c.autosaveWG.Wait()
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	got, err := c.TryEnqueueAndSteer(InboxRequest{Submit: "later"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposition != sessioninbox.DispositionQueuedFollowup || !got.Paused {
		t.Fatalf("receipt = %+v", got)
	}
	if got.SteerRejected != sessioninbox.SteerRejectedInboxPaused {
		t.Fatalf("steerRejected = %q, want %q", got.SteerRejected, sessioninbox.SteerRejectedInboxPaused)
	}
	if got.DispatchGate != sessioninbox.DispatchGatePaused {
		t.Fatalf("dispatchGate = %q, want %q", got.DispatchGate, sessioninbox.DispatchGatePaused)
	}
}

// A queued wake reports the gate holding it, and a later re-query reports the
// gate closed now rather than the one that was closed at enqueue time.
func TestQueuedWakeNamesTheGateHoldingIt(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(session, []byte("{}\n"), 0o644)
	runner := &gatedTurnRunner{started: make(chan struct{}), release: make(chan struct{})}
	c := New(Options{Runner: runner, SessionPath: session, SessionDir: dir, Sink: event.Discard})
	defer c.autosaveWG.Wait()
	defer close(runner.release)

	if _, err := c.TryEnqueueFollowup(InboxRequest{Submit: "first turn"}); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	waitForRunning(t, c)

	got, err := c.TryEnqueueFollowup(InboxRequest{Submit: "room wake", Idempotency: "wake-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got.DispatchGate != sessioninbox.DispatchGateTurnRunning {
		t.Fatalf("dispatchGate = %q, want %q", got.DispatchGate, sessioninbox.DispatchGateTurnRunning)
	}
	looked, found, err := c.LookupInboxReceipt("wake-2")
	if err != nil || !found {
		t.Fatalf("receipt lookup found=%v err=%v", found, err)
	}
	if looked.DispatchGate != sessioninbox.DispatchGateTurnRunning {
		t.Fatalf("looked-up dispatchGate = %q, want %q", looked.DispatchGate, sessioninbox.DispatchGateTurnRunning)
	}
	if looked.ItemID != got.ItemID {
		t.Fatalf("looked-up item = %q, want %q", looked.ItemID, got.ItemID)
	}
}
