package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/sessioninbox"
)

const unreachableAddressError = "session is not open in the desktop window; open it there first"

// An addressed wake lands in the session it names or not at all. Landing in the
// foreground instead is invisible to the sender: a room's wake for one
// conversation would open in another, which is what this pair forbids.
func TestInboxSteerRefusesAStaleAddressInsteadOfMisdelivering(t *testing.T) {
	dir := t.TempDir()
	runner := runtimeStateServeRunner{started: make(chan struct{})}
	foreground := runtimeStateServeController(t, dir, "foreground", runner)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return errors.New(unreachableAddressError) })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, filepath.Join(dir, "gone.jsonl"))
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("a steer naming a session this host cannot reach: status=%d, want 409", response.StatusCode)
	}
	if items := foreground.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("a refused wake admitted %d item(s) into the foreground session", len(items))
	}
	select {
	case <-runner.started:
		t.Fatal("a refused wake started a turn in the foreground session")
	default:
	}
}

// The session this host can honor is the one it delivers to, and the receipt
// says so in its body: a sender must not need a response header to learn it.
func TestInboxAddressedSteerLandsInTheNamedSession(t *testing.T) {
	dir := t.TempDir()
	runner := runtimeStateServeRunner{started: make(chan struct{})}
	foreground := runtimeStateServeController(t, dir, "foreground", runner)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return nil })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	named := agent.CanonicalSessionPath(foreground.SessionPath())
	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, named)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("addressed steer: status=%d, want 202", response.StatusCode)
	}
	receipt := decodeInboxReceipt(t, response)
	if receipt.SessionPath != named || receipt.RequestedSessionPath != named {
		t.Fatalf("receipt sessionPath=%q requestedSessionPath=%q, want both %q",
			receipt.SessionPath, receipt.RequestedSessionPath, named)
	}
	if landed := response.Header.Get(sessionPathHeader); agent.CanonicalSessionPath(landed) != named {
		t.Fatalf("landing header = %q, want %q", landed, named)
	}
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the addressed wake never started a turn")
	}
}

// Activation can report success without moving the delivery target. That case
// must refuse too: admitting the item is the misdelivery this test forbids.
func TestInboxSteerRefusesWhenActivationDoesNotReachTheAddress(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return nil })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, filepath.Join(dir, "elsewhere.jsonl"))
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", response.StatusCode)
	}
	if items := foreground.InboxSnapshot().Items; len(items) != 0 {
		t.Fatalf("a refused wake admitted %d item(s) into the foreground session", len(items))
	}
}

// A wake that names no session is a browser client's ordinary message, and the
// foreground session is then the target by contract. The receipt says so by
// leaving requestedSessionPath empty.
func TestInboxUnaddressedWakeUsesTheForegroundSession(t *testing.T) {
	dir := t.TempDir()
	runner := runtimeStateServeRunner{started: make(chan struct{})}
	foreground := runtimeStateServeController(t, dir, "foreground", runner)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"queued follow-up"}`, "")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("unaddressed follow-up: status=%d, want 202", response.StatusCode)
	}
	receipt := decodeInboxReceipt(t, response)
	if receipt.RequestedSessionPath != "" {
		t.Fatalf("requestedSessionPath = %q, want empty for an unaddressed wake", receipt.RequestedSessionPath)
	}
	if want := agent.CanonicalSessionPath(foreground.SessionPath()); receipt.SessionPath != want {
		t.Fatalf("sessionPath = %q, want the foreground session %q", receipt.SessionPath, want)
	}
}

func postInboxWake(t *testing.T, baseURL, body, session string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/inbox/items", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(session) != "" {
		req.Header.Set(sessionPathHeader, session)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func decodeInboxReceipt(t *testing.T, response *http.Response) sessioninbox.InboxReceipt {
	t.Helper()
	var receipt sessioninbox.InboxReceipt
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	return receipt
}
