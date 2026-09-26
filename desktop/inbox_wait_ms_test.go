package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/provider"
	"reasonix/internal/sessioninbox"
)

// The panel's own queue must answer "how long has this been waiting" the same way
// the by-seq route does: without it, a reader cannot tell a room that is holding
// its tongue from a host that is holding the queue from a slow first token.
func TestInboxViewSaysHowLongTheLineWaited(t *testing.T) {
	const kind = "desktop-wait-ms"
	p := &wakeRecordingProvider{requests: make(chan provider.Request, 4)}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return p, nil })
	isolateDesktopUserDirs(t)
	cfg := config.Default()
	cfg.DefaultModel = "ms/ms-model"
	cfg.Desktop.ProviderAccess = []string{"ms"}
	cfg.Providers = []config.ProviderEntry{{Name: "ms", Kind: kind, Model: "ms-model"}}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// The "no longer queued ⇒ no wait" half is pinned by
	// TestRoomLineAnswersAgreeAcrossHostSurfaces.
	t.Run("a queued line says how long it has waited on both routes", func(t *testing.T) {
		app := NewApp()
		app.ctx = context.Background()
		app.readyHook = func() {}
		tab := modelSettingsBootTab(t, app, "waiting", t.TempDir(), "ms/ms-model")
		ctrl, ok := tab.Ctrl.(*control.Controller)
		if !ok {
			t.Fatalf("controller = %T", tab.Ctrl)
		}
		// Hold the queue so the line stays queued while we look at it.
		ctrl.SetBeforeInboxDispatch(func(*control.Controller) (func(), error) {
			return nil, &control.InboxDispatchRefusal{
				Reason: "这个会话在桌面端已经没有标签页在托管它", Err: control.ErrInboxRuntimeUnpublished,
			}
		})
		if _, err := ctrl.EnqueueInbox(control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
			Extra:       map[string]string{"room.seq": "43"},
			Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
		}); err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		ctrl.NotifyInboxRuntimeReady()
		time.Sleep(30 * time.Millisecond)

		view, err := app.InboxSnapshot(tab.ID)
		if err != nil {
			t.Fatalf("InboxSnapshot: %v", err)
		}
		if len(view.Items) != 1 {
			t.Fatalf("view items = %+v, want one", view.Items)
		}
		item := view.Items[0]
		if item.State != string(sessioninbox.StateQueued) {
			t.Fatalf("state = %q, want queued", item.State)
		}
		if item.WaitMs <= 0 {
			t.Fatalf("waitMs = %d, want how long this line has been waiting", item.WaitMs)
		}
		line, err := app.InboxRoomLine(tab.ID, 43)
		if err != nil || !line.Found || line.Line == nil || line.Line.QueuedForMs <= 0 {
			t.Fatalf("room line = %+v, err = %v, want the same wait on the by-seq route", line, err)
		}
	})
}
