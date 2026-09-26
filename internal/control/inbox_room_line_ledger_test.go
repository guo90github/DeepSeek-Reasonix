package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// "Did my line 43 ever run?" must be answerable from the seq alone. The ack path
// is the only place that still knows the pair, so it remembers the ending there.
func TestSettledRoomLineAnswersBySeqAfterTheLineRan(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	rec, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
		Extra:       map[string]string{"room.seq": "43"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	})
	if err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	if err := c.inbox.store.SetState(rec.ItemID, sessioninbox.StateRunning, ""); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	// Run the real completion path: it must both acknowledge the item and leave the
	// pair behind for the seq query.
	c.inbox.mu.Lock()
	c.inbox.trackActive(rec.ItemID)
	c.inbox.mu.Unlock()
	c.onInboxTurnDone()

	if items := c.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("items = %+v，跑完并确认之后队列该是空的", items)
	}
	line, found := c.InboxRoomLineFor(43)
	if found {
		t.Fatal("跑完的那条不该再算“队列里有”")
	}
	if line.Settled != "acknowledged" {
		t.Fatalf("settled = %q，想要 acknowledged：按 seq 一问就该说出它跑完了", line.Settled)
	}
	if line.SettledAt == "" {
		t.Fatal("settledAt 空：说不出是什么时候结束的")
	}
	if _, found := c.InboxRoomLineFor(44); found {
		t.Fatal("没接过的 seq 不该有账")
	}
}

// The ledger is bounded: the oldest ending is forgotten once the cap is reached,
// and the answer degrades to "no record" rather than to a wrong one.
func TestSettledRoomLineLedgerIsBounded(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	for seq := int64(1); seq <= roomLineSettledLimit+1; seq++ {
		c.noteRoomLineSettled(seq, "acknowledged")
	}
	if _, found := c.InboxRoomLineFor(1); found {
		t.Fatal("最早的那条早该被挤出账本，却还有账")
	}
	line, found := c.InboxRoomLineFor(roomLineSettledLimit + 1)
	if found || line.Settled != "acknowledged" {
		t.Fatalf("line = %+v, found = %v，最新那条该还在账上", line, found)
	}
	if got := len(c.inbox.settledRoomLines); got != roomLineSettledLimit {
		t.Fatalf("账本有 %d 条，上界是 %d", got, roomLineSettledLimit)
	}
}
