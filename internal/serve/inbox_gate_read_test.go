package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// A room only learns the gate today from the receipt of a wake it already
// pushed, or from a per-line ask that needs a seq. Both presume the room posted
// something; this read answers "am I being held?" on its own, which is the answer
// a room needs before it decides whether waking anyone is worth it.
func TestInboxGateAnswersTheHoldWithoutAWake(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sink := &runtimeStateServeSink{states: make(chan event.RuntimeStateSnapshot, 64)}
	foreground := control.New(control.Options{SessionDir: dir, SessionPath: path, Label: "foreground", Sink: sink})
	t.Cleanup(foreground.Close)
	httpServer := httptest.NewServer(New(foreground, nil, config.ServeConfig{}).Handler())
	defer httpServer.Close()

	read := func() map[string]any {
		t.Helper()
		response, err := http.Get(httpServer.URL + "/inbox/gate")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		body := map[string]any{}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body
	}

	// "Nothing is holding you" is an answer too, and an empty queue still gives it.
	open := read()
	if paused, _ := open["paused"].(bool); paused {
		t.Fatalf("body = %+v, want no pause on a fresh queue", open)
	}
	if queued, _ := open["queued"].(float64); queued != 0 {
		t.Fatalf("body = %+v, want an empty queue counted as zero", open)
	}

	if err := foreground.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	if _, err := foreground.EnqueueInbox(control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: "点名", Display: "点名", Raw: "点名",
		Idempotency: "room-wake:chatting:127.0.0.1:8899:71",
	}); err != nil {
		t.Fatal(err)
	}

	held := read()
	if held["gate"] != sessioninbox.GatePaused {
		t.Fatalf("body = %+v, want the paused gate named", held)
	}
	if paused, _ := held["paused"].(bool); !paused {
		t.Fatalf("body = %+v, want the queue's own flag to agree with the gate", held)
	}
	if resumable, _ := held["resumable"].(bool); resumable {
		t.Fatalf("body = %+v, want resumable=false: only a human resumes this", held)
	}
	if queued, _ := held["queued"].(float64); queued != 1 {
		t.Fatalf("body = %+v, want the waiting line counted", held)
	}
	// Its value is timing — an enqueue and a read can land in the same millisecond — so what a
	// room needs from this field is that the answer carries it at all, never that it is > 0.
	if waited, ok := held["oldestQueuedForMs"].(float64); !ok || waited < 0 {
		t.Fatalf("body = %+v, want the wait so far reported, not omitted", held)
	}
	if reason, _ := held["gateReason"].(string); reason != sessioninbox.GateReasonText(sessioninbox.GatePaused) {
		t.Fatalf("body = %+v, want the sentence the room relays verbatim", held)
	}
	if again := read(); again["gate"] != held["gate"] {
		t.Fatalf("re-read = %+v, want the same hold: asking must not change it", again)
	}
}
