package boot

import (
	"encoding/json"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/plugin"
	"reasonix/internal/sessioninbox"
)

// A room line can reach one session through two routes: the hub's long poll,
// whose idempotency key the producing side sends, and this host's push wake,
// whose key is derived here. Until the room settles one key identity the two
// differ, so the same line queues twice. This nails that state: a change to
// either key fails here instead of surfacing as a silent duplicate — or a silent
// fix — once both routes are armed at the same time.
func TestRoomWakeTwoRoutesQueueTheSameLineTwiceToday(t *testing.T) {
	ctrl := wakeTestController(t)
	raw := readRoomWakeFixture(t)
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}

	// chatting/wake.go today: idempotencyKey = chatting-wake-<seq>.
	const producerKey = "chatting-wake-43"
	if _, err := ctrl.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "http",
		Submit: payload.Text, Display: payload.Text, Raw: payload.Text,
		Idempotency: producerKey,
	}); err != nil {
		t.Fatalf("long-poll route enqueue: %v", err)
	}
	inboxWakeHandler(ctrl)(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message", Payload: raw,
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 2 {
		t.Fatalf("inbox items = %d, want 2 while the two routes use different keys", len(items))
	}
	keys := map[string]bool{}
	for _, item := range items {
		keys[item.Idempotency] = true
	}
	if !keys[producerKey] || !keys["room-wake:chatting:127.0.0.1:8899:43"] {
		t.Fatalf("idempotency keys = %v, want the long-poll key and the host-derived room key", keys)
	}
}
