package control

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

func enqueueSteerForMarker(t *testing.T, ctrl *Controller, st *sessioninbox.Store, body, source string) string {
	t.Helper()
	rec, err := ctrl.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentSteer, Submit: body, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(rec.ItemID, sessioninbox.StateSteerAccepted, ""); err != nil {
		t.Fatal(err)
	}
	return rec.ItemID
}

// Guidance pushed in from outside this session has to say so. The injected
// wrapper alone reads as "queued by the user", which is how a remote wake that
// arrived three minutes late was reported as never having been woken
// (measured 2026-09-25 against a real chatting room).
func TestInboxSteerMarksGuidancePushedInFromOutside(t *testing.T) {
	dir := t.TempDir()
	exec := agent.New(nil, nil, agent.NewSession(""), agent.Options{}, event.Discard)
	ctrl := New(Options{
		Runner: exec, Executor: exec, SystemPrompt: "BASE",
		SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.Discard,
	})
	st, err := ctrl.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}

	remoteID := enqueueSteerForMarker(t, ctrl, st, "room says hi", "http")
	text, err := inboxSteerLoader(st, remoteID)()
	if err != nil {
		t.Fatal(err)
	}
	want := inboxWakeMarkerPrefix + "http item=" + remoteID + "]\nroom says hi"
	if text != want {
		t.Fatalf("externally pushed steer = %q, want %q", text, want)
	}

	localID := enqueueSteerForMarker(t, ctrl, st, "typed here", "")
	plain, err := inboxSteerLoader(st, localID)()
	if err != nil {
		t.Fatal(err)
	}
	if plain != "typed here" {
		t.Fatalf("locally queued steer = %q, want the body untouched", plain)
	}
}

// The marker rides the message body — the turn tail. The provider-visible
// prefix must stay byte-identical across turns, so the marker can never reach
// it; a test is the only thing that keeps that true as the prompt grows.
func TestRemoteWakeMarkerStaysOutOfTheSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	exec := agent.New(nil, nil, agent.NewSession(""), agent.Options{}, event.Discard)
	ctrl := New(Options{
		Runner: exec, Executor: exec, SystemPrompt: "BASE",
		SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"), Sink: event.Discard,
	})
	st, err := ctrl.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	enqueueSteerForMarker(t, ctrl, st, "room says hi", "http")

	if ctrl.SystemPrompt() != "BASE" {
		t.Fatalf("system prompt changed with an item in flight: %q", ctrl.SystemPrompt())
	}
	for _, got := range []string{ctrl.SystemPrompt(), controlSystemMessage(ctrl.History())} {
		if strings.Contains(got, inboxWakeMarkerPrefix) {
			t.Fatalf("the provider-visible prefix carries the wake marker: %q", got)
		}
	}
}

// A producer this build does not name yet still gets a marker, as "unknown":
// swallowing it would leave a reader concluding the guidance was queued here.
// Only an item with no source at all prints nothing — that is the local case.
func TestWakeMarkerNamesAnUnlistedSourceAndSkipsTheLocalOne(t *testing.T) {
	if marker := inboxWakeMarker(sessioninbox.InboxItemMeta{ID: "item-1", Source: "carrier-pigeon"}); marker != "[remote wake source=unknown item=item-1]" {
		t.Fatalf("unlisted source produced %q, want the unknown marker", marker)
	}
	if marker := inboxWakeMarker(sessioninbox.InboxItemMeta{ID: "item-1", Source: "push"}); marker != "[remote wake source=push item=item-1]" {
		t.Fatalf("push source produced %q", marker)
	}
	if marker := inboxWakeMarker(sessioninbox.InboxItemMeta{ID: "item-1"}); marker != "" {
		t.Fatalf("an item queued in this process produced %q, want no marker", marker)
	}
}
