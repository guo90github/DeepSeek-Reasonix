package control

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// "Why hasn't he answered?" needs the wait, not just the state: a queued line must
// say how long it has been queued, so a reader can tell a rate-limited room from a
// host holding the queue from a slow first token.
func TestInboxRoomLineSaysHowLongItWaited(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
		Extra:       map[string]string{"room.seq": "43"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	line, found := c.InboxRoomLineFor(43)
	if !found {
		t.Fatal("the queued line is missing")
	}
	if line.State != string(sessioninbox.StateQueued) {
		t.Fatalf("state = %q, want queued while it is still waiting", line.State)
	}
	if line.QueuedForMs <= 0 {
		t.Fatalf("queuedForMs = %d, want how long this line has been waiting", line.QueuedForMs)
	}
	if _, found := c.InboxRoomLineFor(44); found {
		t.Fatal("a seq this session never took in answered as found")
	}
}
