package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// requireGate pins all three diagnostics at once: the enum a sender branches
// on, the sentence it only relays, and the boolean for the gate a human owns.
// The sentence must be non-empty for a named gate: relaying nothing reads as
// "no gate" to a sender.
func requireGate(t *testing.T, rec sessioninbox.InboxReceipt, gate string) {
	t.Helper()
	if rec.Gate != gate {
		t.Fatalf("gate = %q, want %q (receipt %+v)", rec.Gate, gate, rec)
	}
	if want := sessioninbox.GateReasonText(gate); rec.GateReason != want || rec.GateReason == "" {
		t.Fatalf("gateReason = %q, want %q for gate %q", rec.GateReason, want, gate)
	}
	if want := gate == sessioninbox.GateAwaitingAnswer; rec.PendingPrompt != want {
		t.Fatalf("pendingPrompt = %v, want %v for gate %q", rec.PendingPrompt, want, gate)
	}
}

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
	if got.Gate != "" || got.GateReason != "" || got.PendingPrompt {
		t.Fatalf("receipt = %+v, want no gate for a steer nothing is blocking", got)
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
	requireGate(t, got, sessioninbox.GateTurnRunning)
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
	requireGate(t, got, sessioninbox.GatePaused)
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
	requireGate(t, got, sessioninbox.GateTurnRunning)
	looked, found, err := c.LookupInboxReceipt("wake-2")
	if err != nil || !found {
		t.Fatalf("receipt lookup found=%v err=%v", found, err)
	}
	requireGate(t, looked, sessioninbox.GateTurnRunning)
	if looked.ItemID != got.ItemID {
		t.Fatalf("looked-up item = %q, want %q", looked.ItemID, got.ItemID)
	}
}

// The gate a human has to clear themselves is the one a sender must be told
// about most loudly, so it is named and flagged rather than merely queued.
func TestPendingAnswerIsNamedAsTheUsersOwnGate(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(session, []byte("{}\n"), 0o644)
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	defer c.autosaveWG.Wait()
	done, _ := c.approval.registerDecision("bash", "rm -rf build", "needs your OK", true, true)
	defer c.approval.cancel(done)
	if !c.PendingPrompt() {
		t.Fatal("the fixture did not leave a prompt waiting")
	}

	got, err := c.TryEnqueueAndSteer(InboxRequest{Submit: "room wake", Idempotency: "wake-pending"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposition != sessioninbox.DispositionQueuedFollowup {
		t.Fatalf("disposition = %s, want queued_followup", got.Disposition)
	}
	requireGate(t, got, sessioninbox.GateAwaitingAnswer)
	if !got.PendingPrompt {
		t.Fatalf("receipt = %+v, want pendingPrompt for the answer gate", got)
	}
	looked, found, err := c.LookupInboxReceipt("wake-pending")
	if err != nil || !found {
		t.Fatalf("receipt lookup found=%v err=%v", found, err)
	}
	requireGate(t, looked, sessioninbox.GateAwaitingAnswer)
}

// A caller with no session file gets a refusal, not a receipt: the queue has
// nowhere to land, so there is no gate to name and none is invented.
func TestWakeWithoutASessionFileIsRefusedBeforeAdmission(t *testing.T) {
	c := New(Options{SessionDir: t.TempDir(), Sink: event.Discard})
	defer c.autosaveWG.Wait()
	got, err := c.TryEnqueueAndSteer(InboxRequest{Submit: "room wake"})
	if err == nil {
		t.Fatalf("receipt = %+v, want a refusal for a session with nowhere to persist", got)
	}
	if got.ItemID != "" || got.Gate != "" || got.GateReason != "" || got.PendingPrompt {
		t.Fatalf("receipt = %+v, want no receipt and no gate", got)
	}
}
