package boot

import (
	"encoding/json"
	"errors"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/plugin"
	"reasonix/internal/sessioninbox"
)

// A wake's failures are not interchangeable: a key already held by another line
// means the wake was dropped for good, a full or paused inbox means it may still
// land, and anything else is a plain failure. They used to be one warning line,
// which is why a reused room identity looked exactly like an enqueue hiccup.
func TestWakeEnqueueOutcomeNamesTheFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want wakeEnqueueOutcome
	}{
		{"accepted", nil, wakeEnqueueAccepted},
		{"key collision", sessioninbox.ErrIdempotencyConflict, wakeEnqueueKeyCollision},
		{"paused", sessioninbox.ErrPaused, wakeEnqueueRefused},
		{"items full", sessioninbox.ErrCapacityItems, wakeEnqueueRefused},
		{"bytes full", sessioninbox.ErrCapacityBytes, wakeEnqueueRefused},
		{"other", errors.New("store unavailable"), wakeEnqueueFailed},
	}
	for _, tc := range cases {
		if got := wakeEnqueueOutcomeFor(tc.err); got != tc.want {
			t.Fatalf("%s: outcome = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Two different room lines under one key: the second must be reported as a
// collision and must not queue. Nothing on either side used to say so.
func TestWakeCollidingKeyIsReportedAndNotQueued(t *testing.T) {
	ctrl := wakeTestController(t)
	raw := readRoomWakeFixture(t)
	first := plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message", Payload: raw,
	}
	if got := deliverInboxWake(ctrl, first); got != wakeEnqueueAccepted {
		t.Fatalf("first wake outcome = %v, want accepted", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["text"] = "另一条房间行，撞了同一个 seq"
	other, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	second := plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message", Payload: other,
	}
	if got := deliverInboxWake(ctrl, second); got != wakeEnqueueKeyCollision {
		t.Fatalf("colliding wake outcome = %v, want the key-collision class", got)
	}

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1: a colliding line must not queue twice", len(items))
	}
	if items[0].Preview != "@DeepSeek-Reasonix 房间 #43：跨仓 fixture 的冻结载荷" {
		t.Fatalf("queued item = %q, want the first line to survive", items[0].Preview)
	}
}

// Replaying the same wake is not a failure and must not queue twice: the inbox
// folds it onto the item it already holds.
func TestWakeReplayIsAcceptedAndNotDuplicated(t *testing.T) {
	ctrl := wakeTestController(t)
	wake := plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message", Payload: readRoomWakeFixture(t),
	}
	if got := deliverInboxWake(ctrl, wake); got != wakeEnqueueAccepted {
		t.Fatalf("first outcome = %v, want accepted", got)
	}
	if got := deliverInboxWake(ctrl, wake); got != wakeEnqueueAccepted {
		t.Fatalf("replay outcome = %v, want accepted", got)
	}
	if items := ctrl.InboxSnapshot().Items; len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1 after a replay", len(items))
	}
}

// An unattributable wake still stops before the inbox, and a routable one still
// lands: the outcome now says which of the two happened.
func TestWakeSinksKeepTheirDeliveryBehaviour(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := SharedWakeHandler(func(string) *control.Controller { return ctrl })
	handler(plugin.WakeMessage{
		Server: "chatting", Caller: "/sessions/a.jsonl",
		Payload: json.RawMessage(`{"seq":99,"text":"plain"}`),
	})
	if items := ctrl.InboxSnapshot().Items; len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(items))
	}

	dropped := wakeTestController(t)
	SharedWakeHandler(func(string) *control.Controller { return nil })(plugin.WakeMessage{
		Server: "chatting", Caller: "/sessions/gone.jsonl",
		Payload: json.RawMessage(`{"seq":100,"text":"plain"}`),
	})
	if items := dropped.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("an unattributable wake admitted %d item(s)", len(items))
	}
}
