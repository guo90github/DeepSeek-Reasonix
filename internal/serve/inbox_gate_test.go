package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/sessioninbox"
)

// The wake contract has to leave the process on the wire: a sender learns why
// its steer was only queued from the 202 body, not from a log line.
func TestQueuedWakeReceiptCarriesTheGateOverHTTP(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	if err := foreground.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, "")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("a wake into a paused inbox: status=%d, want 202", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	receipt := sessioninbox.InboxReceipt{}
	if err := json.Unmarshal(body, &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt.Disposition != sessioninbox.DispositionQueuedFollowup {
		t.Fatalf("disposition = %s, want queued_followup", receipt.Disposition)
	}
	if receipt.SteerRejected != sessioninbox.SteerRejectedInboxPaused {
		t.Fatalf("steerRejected = %q, want %q", receipt.SteerRejected, sessioninbox.SteerRejectedInboxPaused)
	}
	if receipt.Gate != sessioninbox.GatePaused {
		t.Fatalf("gate = %q, want %q", receipt.Gate, sessioninbox.GatePaused)
	}
	if receipt.GateReason != sessioninbox.GateReasonText(sessioninbox.GatePaused) {
		t.Fatalf("gateReason = %q, want the host's own sentence", receipt.GateReason)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"steerRejected", "gate", "gateReason"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("202 body lost %q: %s", key, body)
		}
	}
	if _, ok := fields["pendingPrompt"]; ok {
		t.Fatalf("a pause claimed the user owed an answer: %s", body)
	}
}

// The same receipt stays silent about the steer when the wake was never one,
// and names the host as the owner of the next kick: the serve host installs a
// publication hook, so the controller cannot see when it will admit the item.
func TestQueuedFollowupWakeNamesTheHostKickOverHTTP(t *testing.T) {
	dir := t.TempDir()
	runner := runtimeStateServeRunner{started: make(chan struct{})}
	foreground := runtimeStateServeController(t, dir, "foreground", runner)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake"}`, "")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("follow-up wake: status=%d, want 202", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	receipt := sessioninbox.InboxReceipt{}
	if err := json.Unmarshal(body, &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt.Gate != sessioninbox.GateHostDispatch {
		t.Fatalf("gate = %q, want %q", receipt.Gate, sessioninbox.GateHostDispatch)
	}
	if receipt.GateReason != sessioninbox.GateReasonText(sessioninbox.GateHostDispatch) {
		t.Fatalf("gateReason = %q, want the host's own sentence", receipt.GateReason)
	}
	if receipt.PendingPrompt {
		t.Fatalf("the host-owned kick claimed the user owed an answer: %s", body)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["steerRejected"]; ok {
		t.Fatalf("a follow-up wake reported a steer refusal: %s", body)
	}
}
