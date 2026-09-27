package boot

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/plugin"
)

func wakeTestController(t *testing.T) *control.Controller {
	t.Helper()
	dir := t.TempDir()
	exec := agent.New(nil, nil, agent.NewSession(""), agent.Options{}, event.Discard)
	return control.New(control.Options{
		Runner: exec, Executor: exec, SystemPrompt: "BASE",
		SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.Discard,
	})
}

// A wake from an armed server lands as guidance in this session's inbox: the
// session is woken without a poll being in flight, which is the whole point of
// the channel. The stored source is also what makes the injected guidance say
// [remote wake source=…] instead of lying about the user having queued it, and a
// payload naming the room's own line reports the room rather than the transport.
func TestServerWakeLandsInTheSessionInbox(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := inboxWakeHandler(ctrl)
	handler(plugin.WakeMessage{
		Server: "room", Method: "notifications/room/room_message",
		Payload: json.RawMessage(`{"seq":43,"text":"Chat room #43: fusion-root mentioned you"}`),
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(items))
	}
	if items[0].Source != "room-wake" {
		t.Fatalf("item source = %q, want room-wake for a payload naming a room line", items[0].Source)
	}
}

// A wake carrying no text is dropped instead of admitted as an empty turn.
func TestServerWakeWithoutTextIsDropped(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := inboxWakeHandler(ctrl)
	handler(plugin.WakeMessage{Server: "room", Payload: json.RawMessage(`{"seq":43}`)})
	handler(plugin.WakeMessage{Server: "room", Payload: json.RawMessage(`not json`)})

	if items := ctrl.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("a textless wake admitted %d item(s)", len(items))
	}
}

// An unwired host (no session yet, or a shared host whose frontend armed no
// resolver) must not panic when a wake arrives: the plugin layer already logs
// the drop.
func TestInboxWakeHandlerToleratesNoController(t *testing.T) {
	installInboxWakeHandler(nil, nil)
	if handler := inboxWakeHandler(nil); handler != nil {
		t.Fatal("a nil controller produced a wake handler")
	}
	if handler := SharedWakeHandler(nil); handler != nil {
		t.Fatal("a nil resolver produced a shared wake handler")
	}
}

// The desktop's host is shared across sessions, so a wake must land in the inbox
// of the session its caller names: there is no foreground session to fall back
// on, and the fallback used to be a logged drop.
func TestSharedHostWakeLandsInTheCallersInbox(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := SharedWakeHandler(func(path string) *control.Controller {
		if path == "/sessions/b.jsonl" {
			return ctrl
		}
		return nil
	})
	if handler == nil {
		t.Fatal("a resolver produced no shared wake handler")
	}
	handler(plugin.WakeMessage{
		Server: "room", Caller: "/sessions/b.jsonl",
		Payload: json.RawMessage(`{"seq":43,"text":"mentioned you"}`),
	})

	items := ctrl.InboxSnapshot().Items
	if len(items) != 1 || items[0].Source != "room-wake" {
		t.Fatalf("inbox items = %+v, want one room-wake item in the caller's session", items)
	}
}

// An unknown caller keeps the visible drop: a host serving several sessions that
// guessed one would deliver this mention into the wrong session's inbox.
func TestSharedHostWakeWithUnknownCallerIsDropped(t *testing.T) {
	ctrl := wakeTestController(t)
	handler := SharedWakeHandler(func(string) *control.Controller { return nil })
	handler(plugin.WakeMessage{
		Server: "room", Caller: "/sessions/gone.jsonl",
		Payload: json.RawMessage(`{"seq":43,"text":"mentioned you"}`),
	})

	if items := ctrl.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("an unattributable wake admitted %d item(s)", len(items))
	}
}
