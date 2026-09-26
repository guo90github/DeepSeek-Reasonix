package boot

import (
	"encoding/json"
	"testing"

	"reasonix/internal/plugin"
)

// The structured room fields travel with the wake so the frontend can badge it
// as a chat-room item; the preview text stays verbatim and unchanged.
func TestServerWakeCarriesRoomMeta(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := inboxWakeHandler(ctrl)
	handler(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message",
		Payload: json.RawMessage(`{"seq":43,"from":"fusion-root","text":"Chat room #43: fusion-root mentioned you","topic":4,"mentions":["reasonix-host"],"kind":"say","origin":"agent","panel":"http://127.0.0.1:8899"}`),
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(items))
	}
	room := items[0].Room
	if room == nil {
		t.Fatal("room meta = nil, want the payload's structured fields")
	}
	if room.Seq != 43 || room.From != "fusion-root" || room.Topic != 4 || room.Kind != "say" || room.Origin != "agent" {
		t.Fatalf("room meta = %+v, want seq=43 from=fusion-root topic=4 kind=say origin=agent", room)
	}
	if room.Panel != "http://127.0.0.1:8899" {
		t.Fatalf("room panel = %q, want http://127.0.0.1:8899", room.Panel)
	}
	if len(room.Mentions) != 1 || room.Mentions[0] != "reasonix-host" {
		t.Fatalf("room mentions = %v, want [reasonix-host]", room.Mentions)
	}
	if items[0].Preview != "Chat room #43: fusion-root mentioned you" {
		t.Fatalf("item body = %q, want the payload text verbatim", items[0].Preview)
	}
}

// A wake without room fields must not grow a room object: non-room pushes stay
// roomless so the frontend never badges them.
func TestServerWakeWithoutRoomFieldsHasNoRoomMeta(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := inboxWakeHandler(ctrl)
	handler(plugin.WakeMessage{
		Server: "other", Method: "notifications/other/push",
		Payload: json.RawMessage(`{"text":"plain push"}`),
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(items))
	}
	if items[0].Room != nil {
		t.Fatalf("room meta = %+v, want nil for a non-room wake", items[0].Room)
	}
}

// One room line is one inbox item: a retried wake with the same server+seq
// lands on the already-queued item instead of queueing a duplicate.
func TestServerWakeRetryIsIdempotent(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := inboxWakeHandler(ctrl)
	wake := plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message",
		Payload: json.RawMessage(`{"seq":43,"from":"fusion-root","text":"Chat room #43: fusion-root mentioned you","topic":4,"kind":"say","origin":"agent"}`),
	}
	handler(wake)
	handler(wake)

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1 after a retried wake", len(items))
	}
	if items[0].Idempotency != "room-wake:chatting:43" {
		t.Fatalf("idempotency = %q, want room-wake:chatting:43", items[0].Idempotency)
	}
}

// A non-http(s) panel value must never reach the frontend's openExternal.
func TestServerWakeDropsNonHTTPPanel(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := inboxWakeHandler(ctrl)
	handler(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message",
		Payload: json.RawMessage(`{"seq":44,"from":"hub","text":"batch done","kind":"say","origin":"agent","panel":"javascript:alert(1)"}`),
	})
	handler(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message",
		Payload: json.RawMessage(`{"seq":45,"from":"hub","text":"batch done","kind":"say","origin":"agent","panel":"file:///C:/windows/system32/calc.exe"}`),
	})
	handler(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message",
		Payload: json.RawMessage(`{"seq":46,"from":"hub","text":"batch done","kind":"say","origin":"agent","panel":"http://127.0.0.1:8899"}`),
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 3 {
		t.Fatalf("inbox items = %d, want 3", len(items))
	}
	if items[0].Room.Panel != "" || items[1].Room.Panel != "" {
		t.Fatalf("non-http panel leaked: %q / %q", items[0].Room.Panel, items[1].Room.Panel)
	}
	if items[2].Room.Panel != "http://127.0.0.1:8899" {
		t.Fatalf("http panel = %q, want http://127.0.0.1:8899", items[2].Room.Panel)
	}
}
