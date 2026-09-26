package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// A reader must not have to infer "the host answered" from an empty field: the
// wire carries it. Three states are pinned, because two of them used to look alike.
func TestRoomLineSaysWhetherTheHostRefusedIt(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	enqueue := func(seq int64) string {
		t.Helper()
		rec, err := c.EnqueueInbox(InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: "点名", Display: "点名", Raw: "点名",
			Extra:       map[string]string{"room.seq": itoaSeq(seq)},
			Idempotency: "room-wake:chatting:127.0.0.1:8899:" + itoaSeq(seq),
		})
		if err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		return rec.ItemID
	}

	// Nobody refused this one: the gate alone says why it is not running.
	plain := enqueue(41)
	if line, found := c.InboxRoomLineFor(41); !found || line.Refused || line.Resumable || line.Reason != "" {
		t.Fatalf("line = %+v，没人拒过的行不该带拒过/续期", line)
	}

	// Refused, and another wake could still lift it.
	liftable := enqueue(42)
	c.noteInboxHostAnswer(liftable, &InboxDispatchRefusal{Reason: "忙", Resumable: true})
	line, found := c.InboxRoomLineFor(42)
	if !found || !line.Refused || !line.Resumable || line.Reason != "忙" {
		t.Fatalf("line = %+v，拒过且可续期这一档没被说出来", line)
	}

	// Refused, and no other wake will help: same reason, no resumable flag.
	stuck := enqueue(43)
	c.noteInboxHostAnswer(stuck, &InboxDispatchRefusal{Reason: "没有标签页在托管它"})
	line, found = c.InboxRoomLineFor(43)
	if !found || !line.Refused || line.Resumable || line.Reason != "没有标签页在托管它" {
		t.Fatalf("line = %+v，拒过但提不起来这一档没被说出来", line)
	}
	if _, _, refused := c.inboxHostRefusalFor(plain); refused {
		t.Fatal("没人拒过的那一条被记成了拒过")
	}
}

func itoaSeq(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
