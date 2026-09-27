package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// A queued room line whose start attempt failed is not "nobody touched it yet":
// the queue was open, the host tried, and the turn itself could not start. That
// was the 2026-09-27 blind spot — the room saw 202 plus idle for five hours
// because nothing distinguished "held" from "never got off the ground".
func TestRoomLineSaysWhyAQueuedTurnNeverStarted(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	const failure = "context exceeds provider limit and compaction failed"
	c.inbox.mu.Lock()
	c.inbox.beforeDispatchSubmit = func(string) error { return errors.New(failure) }
	c.inbox.scheduleDispatchRetry = func(time.Duration, func()) {}
	c.inbox.mu.Unlock()

	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
		Extra:       map[string]string{"room.seq": "43"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	c.maybeDispatchInbox()

	line, found := c.InboxRoomLineFor(43)
	if !found {
		t.Fatal("the queued line is missing")
	}
	if line.State != string(sessioninbox.StateQueued) {
		t.Fatalf("state = %q, want it still queued after the failed start", line.State)
	}
	if line.Gate != sessioninbox.GateStartFailed {
		t.Fatalf("gate = %q, want %q so the reader stops looking for a holder", line.Gate, sessioninbox.GateStartFailed)
	}
	if !strings.Contains(line.Reason, failure) {
		t.Fatalf("reason = %q, want the failure sentence a sender can act on", line.Reason)
	}
	if line.Resumable {
		t.Fatal("a failed start reported as resumable: re-posting the same line would fail the same way")
	}
}

// A started turn clears the recorded failure: the next queued line must not
// inherit an answer about a start that has since succeeded.
func TestStartedTurnClearsTheRecordedStartFailure(t *testing.T) {
	c, runner, done := newInboxDispatchController(t)
	c.noteInboxStartFailure(errors.New("stale failure"))
	if got := c.inboxStartFailure(); got == "" {
		t.Fatal("the fixture did not record a failure")
	}

	if _, err := c.EnqueueInbox(InboxRequest{Submit: "go"}); err != nil {
		t.Fatal(err)
	}
	c.maybeDispatchInbox()
	if got := waitForInboxDispatch(t, c, runner); got != "go" {
		t.Fatalf("dispatched input = %q", got)
	}
	waitForInboxTurnDone(t, c, done)
	if got := c.inboxStartFailure(); got != "" {
		t.Fatalf("start failure = %q after a turn started, want it cleared", got)
	}
}
