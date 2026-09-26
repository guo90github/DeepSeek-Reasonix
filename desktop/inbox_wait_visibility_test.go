package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/sessioninbox"
)

// The one wake that can strand is the one whose session has no tab hosting it.
// The host already answers with a sentence for it; this pins that the answer
// reaches the inbox view instead of stopping at a log line — "the session looks
// dead" and "this wake is waiting for a host" must not look the same.
func TestInboxViewSaysWhyAQueuedWakeIsNotRunning(t *testing.T) {
	isolateDesktopUserDirs(t)
	p := &wakeRecordingProvider{requests: make(chan provider.Request, 4)}
	const kind = "desktop-inbox-wait-test"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return p, nil })
	cfg := config.Default()
	cfg.DefaultModel = "wait/wait-model"
	cfg.Desktop.ProviderAccess = []string{"wait"}
	cfg.Providers = []config.ProviderEntry{{Name: "wait", Kind: kind, Model: "wait-model"}}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app := NewApp()
	app.ctx = context.Background()
	root := t.TempDir()
	ctrl, err := boot.Build(app.ctx, boot.Options{
		Model: "wait/wait-model", WorkspaceRoot: root, SessionDir: desktopSessionDir(root),
		Sink: event.Discard, BeforeInboxDispatch: app.beforeInboxDispatch,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctrl.Close)
	// Deliberately not registered in app.tabs: no tab hosts this session, which is
	// the only shape that strands a wake.
	ctrl.AdoptHistory(ctrl.History(), filepath.Join(ctrl.SessionDir(), "stranded.jsonl"))

	const wake = "房间 #43：chatside 点名了你"
	if _, err := ctrl.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: wake, Display: wake, Raw: wake,
		Extra:       map[string]string{"room.seq": "43", "room.from": "chatside"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	ctrl.NotifyInboxRuntimeReady()
	time.Sleep(300 * time.Millisecond)

	snap := ctrl.InboxSnapshot()
	if len(snap.Items) != 1 {
		t.Fatalf("inbox items = %d, want the wake still queued", len(snap.Items))
	}
	view := inboxSnapshotView(snap, ctrl.InboxDispatchWait)
	item := view.Items[0]
	if !strings.Contains(item.WaitReason, "没有标签页在托管它") {
		t.Fatalf("waitReason = %q, want the host's own sentence for a session it cannot admit", item.WaitReason)
	}
	if item.WaitResumable {
		t.Fatalf("waitResumable = true, want false: reopening the tab is the only way to lift this one")
	}
	if item.WaitGate == "" {
		t.Fatal("waitGate is empty, want the gate that is holding the wake")
	}
	if len(p.requests) != 0 {
		t.Fatalf("a stranded wake started %d turn(s), want none", len(p.requests))
	}
}

// blockingWakeProvider holds a turn open until the test releases it.
type blockingWakeProvider struct {
	started chan provider.Request
	release chan struct{}
}

func (p *blockingWakeProvider) Name() string { return "desktop-inbox-gate-test" }

func (p *blockingWakeProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	select {
	case p.started <- req:
	default:
	}
	select {
	case <-p.release:
	case <-ctx.Done():
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// A blocking provider keeps one turn in flight, which is how a follow-up ends up
// queued behind it. Reading the tab's inbox through the app must then say which
// gate holds that item: without the app handing the view a way to ask, a queued
// item and an idle inbox look exactly alike.
func TestInboxSnapshotViewExplainsATurnRunningGate(t *testing.T) {
	isolateDesktopUserDirs(t)
	p := &blockingWakeProvider{started: make(chan provider.Request, 1), release: make(chan struct{})}
	const kind = "desktop-inbox-gate-test"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return p, nil })
	cfg := config.Default()
	cfg.DefaultModel = "gate/gate-model"
	cfg.Desktop.ProviderAccess = []string{"gate"}
	cfg.Providers = []config.ProviderEntry{{Name: "gate", Kind: kind, Model: "gate-model"}}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.readyHook = func() {}
	tab := modelSettingsBootTab(t, app, "busy", t.TempDir(), "gate/gate-model")
	ctrl, ok := tab.Ctrl.(*control.Controller)
	if !ok {
		t.Fatalf("controller = %T", tab.Ctrl)
	}
	go func() { _ = ctrl.Run(context.Background(), "start the turn") }()
	select {
	case <-p.started:
	case <-time.After(30 * time.Second):
		t.Fatal("the turn never reached the provider")
	}
	if _, err := ctrl.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentFollowup, Source: "http",
		Submit: "queued behind the turn", Display: "queued behind the turn", Raw: "queued behind the turn",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}
	close(p.release)

	view, err := app.InboxSnapshot(tab.ID)
	if err != nil {
		t.Fatalf("InboxSnapshot: %v", err)
	}
	var queued *InboxItemView
	for i := range view.Items {
		if view.Items[i].State == string(sessioninbox.StateQueued) {
			queued = &view.Items[i]
		}
	}
	if queued == nil {
		t.Fatalf("items = %+v, want the follow-up still queued", view.Items)
	}
	if queued.WaitGate == "" {
		t.Fatal("waitGate is empty: the app did not hand the view a way to ask what holds this item")
	}
}
