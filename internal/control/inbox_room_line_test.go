package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// The push route has no reply channel, so a sender asks the host by seq instead
// of assuming the line landed. "Not found" is an answer of its own: it says this
// session never took that line in.
func TestInboxRoomLineAnswersBySeq(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	if _, found := c.InboxRoomLineFor(43); found {
		t.Fatal("a seq with no item answered as found, which would read as delivered")
	}
	if _, found := c.InboxRoomLineFor(0); found {
		t.Fatal("seq 0 answered as found")
	}

	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
		Extra:       map[string]string{"room.seq": "43", "room.from": "chatside"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}

	line, found := c.InboxRoomLineFor(43)
	if !found {
		t.Fatal("seq 43 answered as not found right after it was queued")
	}
	if line.ItemID == "" || line.Source != "push" || line.State == "" {
		t.Fatalf("line = %+v, want the queued push item with its durable state", line)
	}
	if _, found := c.InboxRoomLineFor(44); found {
		t.Fatal("a different seq answered with the item queued for 43")
	}
}
