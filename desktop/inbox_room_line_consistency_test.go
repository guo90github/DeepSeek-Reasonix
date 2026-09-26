package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/provider"
	"reasonix/internal/sessioninbox"
)

// One room line must get one answer from the host, whichever surface is asked:
// the item's own state, the controller's dispatch wait, the desktop's inbox view
// and the bridge's by-seq lookup. A surface that decides for itself is how two
// readers end up disagreeing about whether a mention landed.
func TestRoomLineAnswersAgreeAcrossHostSurfaces(t *testing.T) {
	const kind = "desktop-room-line-consistency"
	p := &wakeRecordingProvider{requests: make(chan provider.Request, 4)}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return p, nil })
	isolateDesktopUserDirs(t)
	cfg := config.Default()
	cfg.DefaultModel = "line/line-model"
	cfg.Desktop.ProviderAccess = []string{"line"}
	cfg.Providers = []config.ProviderEntry{{Name: "line", Kind: kind, Model: "line-model"}}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	const refusal = "这个会话在桌面端已经没有标签页在托管它（标签页已关闭或已分离）"

	t.Run("a refused line reads the same on every surface", func(t *testing.T) {
		app := NewApp()
		app.ctx = context.Background()
		app.readyHook = func() {}
		tab := modelSettingsBootTab(t, app, "refused", t.TempDir(), "line/line-model")
		ctrl, ok := tab.Ctrl.(*control.Controller)
		if !ok {
			t.Fatalf("controller = %T", tab.Ctrl)
		}
		ctrl.SetBeforeInboxDispatch(func(*control.Controller) (func(), error) {
			return nil, &control.InboxDispatchRefusal{
				Reason: refusal, Resumable: false, Err: control.ErrInboxRuntimeUnpublished,
			}
		})
		if _, err := ctrl.EnqueueInbox(control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: "房间 #43 点名了你", Display: "房间 #43 点名了你", Raw: "房间 #43 点名了你",
			Extra:       map[string]string{"room.seq": "43", "room.from": "chatside"},
			Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
		}); err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		ctrl.NotifyInboxRuntimeReady()
		// The refusal reaches the queue through an async admission pass.
		var want control.InboxRoomLine
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			if line, found := ctrl.InboxRoomLineFor(43); found && line.Reason == refusal {
				want = line
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if want.Reason != refusal {
			t.Fatalf("the refusal never reached the queue: %+v", want)
		}

		items := ctrl.InboxSnapshot().Items
		if len(items) != 1 || items[0].State != sessioninbox.StateQueued {
			t.Fatalf("inbox items = %+v, want the refused line still queued", items)
		}
		gate, reason, resumable, ok := ctrl.InboxDispatchWait(items[0].ID)
		if !ok || gate != want.Gate || reason != want.Reason || resumable != want.Resumable {
			t.Fatalf("dispatch wait = (%q, %q, %v), room line = (%q, %q, %v): the two surfaces disagree",
				gate, reason, resumable, want.Gate, want.Reason, want.Resumable)
		}
		view, err := app.InboxSnapshot(tab.ID)
		if err != nil {
			t.Fatalf("InboxSnapshot: %v", err)
		}
		if len(view.Items) != 1 {
			t.Fatalf("view items = %+v, want one", view.Items)
		}
		item := view.Items[0]
		if item.WaitGate != want.Gate || item.WaitReason != want.Reason || item.WaitResumable != want.Resumable {
			t.Fatalf("inbox view = (%q, %q, %v), room line = (%q, %q, %v): the two surfaces disagree",
				item.WaitGate, item.WaitReason, item.WaitResumable, want.Gate, want.Reason, want.Resumable)
		}
		line, err := app.InboxRoomLine(tab.ID, 43)
		if err != nil || !line.Found || line.Line == nil {
			t.Fatalf("InboxRoomLine = %+v, err = %v", line, err)
		}
		if line.Line.Gate != want.Gate || line.Line.Reason != want.Reason || line.Line.Resumable != want.Resumable {
			t.Fatalf("bridge = (%q, %q, %v), room line = (%q, %q, %v): the two surfaces disagree",
				line.Line.Gate, line.Line.Reason, line.Line.Resumable, want.Gate, want.Reason, want.Resumable)
		}
		if !strings.Contains(want.Reason, "没有标签页在托管它") {
			t.Fatalf("room line reason = %q, want the host's own sentence", want.Reason)
		}
	})

	t.Run("a line that started reads as no wait anywhere", func(t *testing.T) {
		app := NewApp()
		app.ctx = context.Background()
		app.readyHook = func() {}
		tab := modelSettingsBootTab(t, app, "started", t.TempDir(), "line/line-model")
		ctrl, ok := tab.Ctrl.(*control.Controller)
		if !ok {
			t.Fatalf("controller = %T", tab.Ctrl)
		}
		if _, err := ctrl.EnqueueInbox(control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: "房间 #44 点名了你", Display: "房间 #44 点名了你", Raw: "房间 #44 点名了你",
			Extra:       map[string]string{"room.seq": "44", "room.from": "chatside"},
			Idempotency: "room-wake:chatting:127.0.0.1:8899:44",
		}); err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		ctrl.NotifyInboxRuntimeReady()
		select {
		case <-p.requests:
		case <-time.After(30 * time.Second):
			t.Fatal("the queued line never reached the provider")
		}

		var item *InboxItemView
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			view, err := app.InboxSnapshot(tab.ID)
			if err != nil {
				t.Fatalf("InboxSnapshot: %v", err)
			}
			for i := range view.Items {
				if view.Items[i].Room != nil && view.Items[i].Room.Seq == 44 {
					item = &view.Items[i]
				}
			}
			if item != nil && item.State != string(sessioninbox.StateQueued) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if item == nil {
			t.Fatal("the started line is missing from the inbox view")
		}
		if item.WaitGate != "" || item.WaitReason != "" || item.WaitResumable {
			t.Fatalf("inbox view = %+v, want no wait claimed for a line that started", item)
		}
		line, err := app.InboxRoomLine(tab.ID, 44)
		if err != nil || !line.Found || line.Line == nil {
			t.Fatalf("InboxRoomLine = %+v, err = %v", line, err)
		}
		if line.Line.Gate != "" || line.Line.Reason != "" || line.Line.Resumable {
			t.Fatalf("bridge = %+v, want no wait claimed for a line that started", line.Line)
		}
	})
}
