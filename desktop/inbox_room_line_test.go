package main

import (
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/sessioninbox"
)

// The room asks "what became of line 43?" and gets one answer, from the query that
// already decides it. A seq this session never took in must read as found=false,
// not as an empty line and not as delivered.
func TestInboxRoomLineAnswersForALocalTab(t *testing.T) {
	isolateDesktopUserDirs(t)
	oldRef, _ := configureSwitchableDefaultModels(t)
	app := NewApp()
	app.ctx = t.Context()
	app.readyHook = func() {}
	tab := modelSettingsBootTab(t, app, "room", t.TempDir(), oldRef)
	ctrl, ok := tab.Ctrl.(*control.Controller)
	if !ok {
		t.Fatalf("controller = %T", tab.Ctrl)
	}

	if view, err := app.InboxRoomLine(tab.ID, 43); err != nil || view.Found {
		t.Fatalf("before any line: view = %+v, err = %v, want found=false", view, err)
	}

	if _, err := ctrl.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
		Extra:       map[string]string{"room.seq": "43", "room.from": "chatside"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}

	view, err := app.InboxRoomLine(tab.ID, 43)
	if err != nil {
		t.Fatalf("InboxRoomLine: %v", err)
	}
	if !view.Found || view.Line == nil || view.Line.State == "" || view.Line.Source != "push" {
		t.Fatalf("view = %+v, want the queued push line with its state", view)
	}
	// The gate names what holds the queue — a desktop host holds it until it
	// publishes, so a gate here is normal. What must stay empty is the host's own
	// refusal: no host said this line cannot run.
	if view.Line.Reason != "" || view.Line.Resumable {
		t.Fatalf("view = %+v, want no refusal recorded for a line the host has not rejected", view)
	}

	other, err := app.InboxRoomLine(tab.ID, 44)
	if err != nil || other.Found {
		t.Fatalf("other line: view = %+v, err = %v, want found=false", other, err)
	}
}
