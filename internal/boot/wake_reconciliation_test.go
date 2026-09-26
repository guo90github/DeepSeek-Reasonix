package boot

import (
	"encoding/json"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/plugin"
	"reasonix/internal/sessioninbox"
)

// resolveRoomOnly answers for exactly one session path, which is what the desktop
// resolver does: a wake naming nothing may not be handed to somebody else.
func resolveRoomOnly(ctrl *control.Controller, path string) func(string) *control.Controller {
	return func(got string) *control.Controller {
		if got == path {
			return ctrl
		}
		return nil
	}
}

// This file is the host half of the wake reconciliation table: for one room line,
// what the host records at each step, in the vocabulary the room can read. The
// point it pins is easy to get wrong from the room's side — a push wake reported
// as "delivered" means the host ACCEPTED it into the inbox, not that the session
// ran; only the dispatch wait says whether the turn actually started.
func TestWakeReconciliationHostSideRecords(t *testing.T) {
	t.Run("routable push is accepted into the inbox", func(t *testing.T) {
		ctrl := wakeTestController(t)
		handler := SharedWakeHandler(resolveRoomOnly(ctrl, "/sessions/a.jsonl"))
		got := handler(plugin.WakeMessage{
			Server: "chatting", Caller: "/sessions/a.jsonl", Payload: readRoomWakeFixture(t),
		})
		if got != plugin.WakeDelivered {
			t.Fatalf("outcome = %q, want %q", got, plugin.WakeDelivered)
		}
		items := ctrl.InboxSnapshot().Items
		if len(items) != 1 || items[0].Source != "push" {
			t.Fatalf("inbox items = %+v, want one push item", items)
		}
		if items[0].Idempotency != "room-wake:chatting:127.0.0.1:8899:43" {
			t.Fatalf("idempotency = %q, want the host-derived room key", items[0].Idempotency)
		}
	})

	t.Run("wake with no caller is unroutable and queues nothing", func(t *testing.T) {
		ctrl := wakeTestController(t)
		handler := SharedWakeHandler(resolveRoomOnly(ctrl, "/sessions/a.jsonl"))
		got := handler(plugin.WakeMessage{Server: "chatting", Payload: readRoomWakeFixture(t)})
		if got != plugin.WakeUnroutable {
			t.Fatalf("outcome = %q, want %q", got, plugin.WakeUnroutable)
		}
		if items := ctrl.InboxSnapshot().Items; len(items) != 0 {
			t.Fatalf("inbox items = %d, want none: nothing may be guessed for an unroutable wake", len(items))
		}
	})

	t.Run("a caller naming no session is unroutable too", func(t *testing.T) {
		ctrl := wakeTestController(t)
		handler := SharedWakeHandler(resolveRoomOnly(ctrl, "/sessions/a.jsonl"))
		got := handler(plugin.WakeMessage{
			Server: "chatting", Caller: "/sessions/gone.jsonl", Payload: readRoomWakeFixture(t),
		})
		if got != plugin.WakeUnroutable {
			t.Fatalf("outcome = %q, want %q", got, plugin.WakeUnroutable)
		}
	})

	t.Run("the same key carrying another line is a collision", func(t *testing.T) {
		ctrl := wakeTestController(t)
		raw := readRoomWakeFixture(t)
		if got := deliverInboxWake(ctrl, plugin.WakeMessage{Server: "chatting", Payload: raw}); got != wakeEnqueueAccepted {
			t.Fatalf("first outcome = %v, want accepted", got)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		payload["text"] = "另一条房间行"
		other, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		wake := plugin.WakeMessage{Server: "chatting", Payload: other}
		if got := wakeOutcomeName(deliverInboxWake(ctrl, wake)); got != plugin.WakeKeyCollision {
			t.Fatalf("outcome = %q, want %q", got, plugin.WakeKeyCollision)
		}
		if items := ctrl.InboxSnapshot().Items; len(items) != 1 {
			t.Fatalf("inbox items = %d, want 1: the colliding line must not queue", len(items))
		}
	})

	t.Run("a host that cannot admit keeps the wake queued and says why", func(t *testing.T) {
		ctrl := wakeTestController(t)
		const refusal = "这个会话在桌面端已经没有标签页在托管它（标签页已关闭或已分离）"
		ctrl.SetBeforeInboxDispatch(func(*control.Controller) (func(), error) {
			return nil, &control.InboxDispatchRefusal{
				Reason: refusal, Resumable: false, Err: control.ErrInboxRuntimeUnpublished,
			}
		})
		handler := SharedWakeHandler(resolveRoomOnly(ctrl, "/sessions/a.jsonl"))
		if got := handler(plugin.WakeMessage{
			Server: "chatting", Caller: "/sessions/a.jsonl", Payload: readRoomWakeFixture(t),
		}); got != plugin.WakeDelivered {
			t.Fatalf("push outcome = %q, want %q — accepted into the inbox is not started", got, plugin.WakeDelivered)
		}
		ctrl.NotifyInboxRuntimeReady()

		items := ctrl.InboxSnapshot().Items
		if len(items) != 1 || items[0].State != sessioninbox.StateQueued {
			t.Fatalf("inbox items = %+v, want the wake still queued", items)
		}
		// The refusal reaches the queue through an async admission pass, so wait
		// for it rather than sampling once.
		var gate, reason string
		var resumable, ok bool
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			gate, reason, resumable, ok = ctrl.InboxDispatchWait(items[0].ID)
			if reason == refusal {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !ok || gate == "" || reason != refusal || resumable {
			t.Fatalf("dispatch wait = (%q, %q, %v, %v), want the host's refusal with no resumable path",
				gate, reason, resumable, ok)
		}
	})

	t.Run("the long-poll route is not a push outcome at all", func(t *testing.T) {
		ctrl := wakeTestController(t)
		if _, err := ctrl.EnqueueInbox(control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "http",
			Submit: "房间行（长轮询路）", Display: "房间行（长轮询路）", Raw: "房间行（长轮询路）",
			Idempotency: "chatting-wake-43",
		}); err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		items := ctrl.InboxSnapshot().Items
		if len(items) != 1 || items[0].Source != "http" {
			t.Fatalf("inbox items = %+v, want one http item", items)
		}
		if items[0].Idempotency != "chatting-wake-43" {
			t.Fatalf("idempotency = %q, want the key the producing side sent", items[0].Idempotency)
		}
	})
}

// A controller without a persisted session cannot own a durable inbox, which is
// another shape a pushed wake can end in. It must not read as a delivery.
func TestWakeWithoutSessionPathFailsClosed(t *testing.T) {
	exec := agent.New(nil, nil, agent.NewSession(""), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{
		Runner: exec, Executor: exec, SystemPrompt: "BASE",
		Sink: event.Discard,
	})
	t.Cleanup(ctrl.Close)

	got := wakeOutcomeName(deliverInboxWake(ctrl, plugin.WakeMessage{
		Server: "chatting", Payload: readRoomWakeFixture(t),
	}))
	if got == plugin.WakeDelivered {
		t.Fatalf("a controller with no session path reported %q", got)
	}
	if items := ctrl.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("inbox items = %d, want none", len(items))
	}
}
