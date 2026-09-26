package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// A line that ran is acknowledged, and an acknowledged item leaves the queue — so
// the by-seq answer alone cannot tell "ran and finished" from "never took it in".
// This pins both halves of the honest pair: the queue says it is gone, the receipt
// says who took it and how it ended.
func TestRoomLineThatRanLeavesTheQueueButKeepsItsReceipt(t *testing.T) {
	const key = "room-wake:chatting:127.0.0.1:8899:43"
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
		Idempotency: key,
	})
	if err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	if line, found := c.InboxRoomLineFor(43); !found || line.State != string(sessioninbox.StateQueued) {
		t.Fatalf("line = %+v, found = %v，想要排队中的那条", line, found)
	}

	// Exactly what a finished turn does: the item runs, then its acknowledgement
	// removes it from the queue.
	st := c.inbox.store
	if st == nil {
		t.Fatal("控制器没有队列存储")
	}
	if err := st.SetState(rec.ItemID, sessioninbox.StateRunning, ""); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	if err := st.AckDequeue(rec.ItemID); err != nil {
		t.Fatalf("AckDequeue: %v", err)
	}

	if _, found := c.InboxRoomLineFor(43); found {
		t.Fatal("跑完并确认之后，队列里不该还有这一条")
	}
	receipt, ok, err := c.LookupInboxReceipt(key)
	if err != nil {
		t.Fatalf("LookupInboxReceipt: %v", err)
	}
	if !ok {
		t.Fatalf("回执不见了：键 %q 查不到，于是“跑过”就真的只剩“没接过”一种读法", key)
	}
	if receipt.Settled != "acknowledged" {
		t.Fatalf("settled = %q，想要 acknowledged：跑完的那条得说得出“怎么结束的”", receipt.Settled)
	}
	if receipt.SettledAt == "" {
		t.Fatal("settledAt 空：回执说不出它是什么时候结束的")
	}
}
