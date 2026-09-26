package boot

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/sessioninbox"
)

// The wake chain ends at the provider boundary: a room wake queued while the
// session is idle must start its own turn, and the model must see it marked as
// arriving from outside. A session that reads a mention as its owner typing is
// the failure this path exists to prevent. The marker rides the message body —
// the turn tail — because the system-prompt prefix has to stay byte-stable for
// prefix caching.
func TestEffectRoomWakeStartsATurnAndReachesTheProvider(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	const kind = "boot-effect-room-wake"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	// The wake arrives in the state a room cares about: a session that has
	// already worked and is now idle.
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	served := len(rec.requests())
	// A room wake is addressed at a persisted session (that is how the landing
	// session is named), so this session must own a path before it can be woken.
	ctrl.EnsureSessionPath()

	const wake = "房间 #43：chatside 点名了你（跨仓 fixture 的冻结载荷）"
	if _, err := ctrl.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: wake, Display: wake, Raw: wake,
		Extra:       map[string]string{"room.seq": "43", "room.from": "chatside", "room.panel": "http://127.0.0.1:8899"},
		Idempotency: "room-wake:chatting:127.0.0.1:8899:43",
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}

	// Queueing is not waking: admission waits for the host to publish this runtime,
	// so an unhosted mention stays queued until a frontend says it is ready. Pin it,
	// because a wake that silently sits queued looks like a session that stopped.
	time.Sleep(200 * time.Millisecond)
	if got := len(rec.requests()); got != served {
		t.Fatalf("a queued wake started a turn before the host published the runtime: requests %d → %d", served, got)
	}
	ctrl.NotifyInboxRuntimeReady()

	req := waitForWakeRequest(t, rec, served)
	var body strings.Builder
	for _, msg := range req.Messages {
		body.WriteString(msg.Content)
		body.WriteByte('\n')
	}
	text := body.String()
	if !strings.Contains(text, "[remote wake source=push item=") {
		t.Fatalf("the wake reached the model without its source marker:\n%s", text)
	}
	if !strings.Contains(text, wake) {
		t.Fatalf("the provider request does not carry the wake body:\n%s", text)
	}
	if sys := systemMessage(req.Messages); strings.Contains(sys, wake) || strings.Contains(sys, "remote wake source=") {
		t.Fatalf("the wake moved into the system-prompt prefix, which must stay byte-stable:\n%s", sys)
	}
}

func waitForWakeRequest(t *testing.T, rec *effectRecordingProvider, served int) provider.Request {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if reqs := rec.requests(); len(reqs) > served {
			return reqs[len(reqs)-1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the wake did not start a turn: requests stayed at %d", served)
	return provider.Request{}
}
