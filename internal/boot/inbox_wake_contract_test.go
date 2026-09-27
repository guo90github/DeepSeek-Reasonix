package boot

import (
	"encoding/json"
	"testing"

	"reasonix/internal/plugin"
)

// The wake payload is a frozen cross-repo contract
// ({seq,from,text,topic,mentions[],kind,origin}). The host takes text and
// nothing else: it does not re-derive mentions, and the other fields must not
// change what lands in the inbox.
func TestWakePayloadContractLandsTextVerbatim(t *testing.T) {
	ctrl := wakeTestController(t)
	inboxWakeHandler(ctrl)(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message",
		Payload: json.RawMessage(`{"seq":43,"from":"fusion-root","text":"Chat room #43: fusion-root mentioned you","topic":4,"mentions":["reasonix-host"],"kind":"say","origin":"agent"}`),
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(items))
	}
	if items[0].Preview != "Chat room #43: fusion-root mentioned you" {
		t.Fatalf("item body = %q, want the payload text verbatim", items[0].Preview)
	}
	if items[0].Source != "room-wake" {
		t.Fatalf("item source = %q, want room-wake for a payload naming a room line", items[0].Source)
	}
}
