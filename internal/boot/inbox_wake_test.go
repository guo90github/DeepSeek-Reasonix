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

// A wake from an armed server lands as guidance in this session's inbox with
// source=push: the session is woken without a poll being in flight, which is
// the whole point of the channel. The stored source is also what makes the
// injected guidance say [remote wake source=push …] instead of lying about the
// user having queued it.
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
	if items[0].Source != "push" {
		t.Fatalf("item source = %q, want push", items[0].Source)
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

// An unwired host (shared host, or no session yet) must not panic when a wake
// arrives: the plugin layer already logs the drop.
func TestInboxWakeHandlerToleratesNoController(t *testing.T) {
	installInboxWakeHandler(nil, nil)
	if handler := inboxWakeHandler(nil); handler != nil {
		t.Fatal("a nil controller produced a wake handler")
	}
}
