package control

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// The read-only answer has to cover both ways a queue stops moving, and it has to
// stay a read: asking a second time must not consume the failure it reports.
func TestInboxGateNamesAFailedStartWithoutPosting(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	const cause = "context exceeds provider limit and compaction failed"
	c.noteInboxStartFailure(errors.New(cause))
	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentFollowup, Submit: "还在等", Idempotency: "gate-1",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	for range 2 {
		gate := c.InboxGate()
		if gate.Gate != sessioninbox.GateStartFailed {
			t.Fatalf("gate = %+v, want %q", gate, sessioninbox.GateStartFailed)
		}
		if gate.GateReason != cause || gate.StartFailure != cause {
			t.Fatalf("gate = %+v, want the failure as the sentence a reader acts on", gate)
		}
		if gate.Resumable {
			t.Fatal("a failed start reported as resumable: another post cannot lift it")
		}
		if gate.Queued != 1 || gate.OldestQueuedForMs <= 0 {
			t.Fatalf("gate = %+v, want the waiting line counted with its wait", gate)
		}
		if gate.Paused || gate.Readonly {
			t.Fatalf("gate = %+v, want the queue's own flags false here", gate)
		}
	}
}
