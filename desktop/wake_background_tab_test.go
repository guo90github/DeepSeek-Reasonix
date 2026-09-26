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

type wakeRecordingProvider struct {
	requests chan provider.Request
}

func (p *wakeRecordingProvider) Name() string { return "desktop-wake-test" }

func (p *wakeRecordingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	select {
	case p.requests <- req:
	default:
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// A room wake must start a turn in the session it was addressed at even when
// that session's tab is not the one in front: the desktop admits a queued wake
// on the tab that owns the controller, not on what the user is looking at. That
// is the "background session stays reachable" half of staying online.
func TestWakeStartsATurnInANonForegroundTab(t *testing.T) {
	isolateDesktopUserDirs(t)
	p := &wakeRecordingProvider{requests: make(chan provider.Request, 4)}
	const kind = "desktop-wake-test"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return p, nil })
	cfg := config.Default()
	cfg.DefaultModel = "wake/wake-model"
	cfg.Desktop.ProviderAccess = []string{"wake"}
	cfg.Providers = []config.ProviderEntry{{Name: "wake", Kind: kind, Model: "wake-model"}}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.readyHook = func() {}
	modelSettingsBootTab(t, app, "front", t.TempDir(), "wake/wake-model")
	backTab := modelSettingsBootTab(t, app, "back", t.TempDir(), "wake/wake-model")

	backCtrl, ok := backTab.Ctrl.(*control.Controller)
	if !ok {
		t.Fatalf("back tab controller = %T, want *control.Controller", backTab.Ctrl)
	}
	const wake = "房间 #43：chatside 点名了你（跨仓 fixture 的冻结载荷）"
	if _, err := backCtrl.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: wake, Display: wake, Raw: wake,
		Extra:       map[string]string{"room.seq": "43", "room.from": "chatside", "room.panel": "http://127.0.0.1:8899"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	backCtrl.NotifyInboxRuntimeReady()

	select {
	case req := <-p.requests:
		var body strings.Builder
		for _, msg := range req.Messages {
			body.WriteString(msg.Content)
			body.WriteByte('\n')
		}
		text := body.String()
		if !strings.Contains(text, "[remote wake source=push item=") {
			t.Fatalf("the background tab's turn reached the model without the wake marker:\n%s", text)
		}
		if !strings.Contains(text, wake) {
			t.Fatalf("the background tab's turn does not carry the wake body:\n%s", text)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the wake did not start a turn in the background tab")
	}
}
