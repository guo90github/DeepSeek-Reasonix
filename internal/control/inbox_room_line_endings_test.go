package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// Every way a room line leaves the queue must be answerable by seq alone: ran and
// finished, cancelled before it ran, or removed outright. The ledger only knows the
// pair if it is read before the item goes, so each exit is pinned here.
func TestRoomLineEndingsAreRememberedBySeq(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	enqueue := func(seq int64, source string) string {
		t.Helper()
		rec, err := c.EnqueueInbox(InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: source,
			Submit: "点名", Display: "点名", Raw: "点名",
			Extra:       map[string]string{"room.seq": itoaSeq(seq)},
			Idempotency: "room-wake:chatting:127.0.0.1:8899:" + itoaSeq(seq),
		})
		if err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		return rec.ItemID
	}

	// Removed outright.
	deleted := enqueue(51, "push")
	if err := c.DeleteInboxItem(deleted); err != nil {
		t.Fatalf("DeleteInboxItem: %v", err)
	}
	if line, found := c.InboxRoomLineFor(51); found || line.Settled != "deleted" {
		t.Fatalf("line = %+v, found = %v，被删掉的那条该说出 deleted", line, found)
	}

	// Cancelled while still pending, by the frontend that owns it.
	cancelled := enqueue(52, "desktop:tab-a")
	result, err := c.CancelWithInboxItemsResult([]string{cancelled}, "desktop:tab-a")
	if err != nil {
		t.Fatalf("CancelWithInboxItemsResult: %v", err)
	}
	if len(result.DiscardedItemIDs) != 1 {
		t.Fatalf("cancel did not discard the item: %+v", result)
	}
	if line, found := c.InboxRoomLineFor(52); found || line.Settled != "discarded" {
		t.Fatalf("line = %+v, found = %v，被取消的那条该说出 discarded", line, found)
	}

	// Ran and finished: the acknowledgement path.
	ran := enqueue(53, "push")
	st := c.inbox.store
	if err := st.SetState(ran, sessioninbox.StateRunning, ""); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	c.inbox.mu.Lock()
	c.inbox.trackActive(ran)
	c.inbox.mu.Unlock()
	c.onInboxTurnDone()
	if line, found := c.InboxRoomLineFor(53); found || line.Settled != "acknowledged" {
		t.Fatalf("line = %+v, found = %v，跑完的那条该说出 acknowledged", line, found)
	}

	// A line nobody ever took in still says nothing beyond "not here".
	if line, found := c.InboxRoomLineFor(54); found || line.Settled != "" || line.SettledAt != "" {
		t.Fatalf("line = %+v, found = %v，没接过的 seq 不该有账", line, found)
	}
}
