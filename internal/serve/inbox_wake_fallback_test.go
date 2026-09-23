package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
)

const staleAddressError = "session is not open in the desktop window; open it there first"

// A steer whose addressed session is not open here must still be delivered. The
// sending session captured that path when its MCP child spawned, so a window
// whose tabs have moved on since then would refuse every wake with 409 forever —
// which is exactly how a real chatting room went silent (measured 2026-09-23).
func TestInboxSteerFallsBackWhenAddressedSessionIsNotOpen(t *testing.T) {
	dir := t.TempDir()
	runner := runtimeStateServeRunner{started: make(chan struct{})}
	foreground := runtimeStateServeController(t, dir, "foreground", runner)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return errors.New(staleAddressError) })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, filepath.Join(dir, "gone.jsonl"))
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("a stale address must not kill the wake: status=%d, want 202", response.StatusCode)
	}
	landed := response.Header.Get(sessionPathHeader)
	if agent.CanonicalSessionPath(landed) != agent.CanonicalSessionPath(foreground.SessionPath()) {
		t.Fatalf("landing header = %q, want the foreground session %q", landed, foreground.SessionPath())
	}
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the degraded wake never started a turn in the foreground session")
	}
}

// A follow-up names its session like /submit does, and that refusal is the
// honest answer: a browser client addressing a session the host cannot show is a
// client bug, not a lost wake. Only the remote wake degrades.
func TestInboxFollowupStillRefusesAnUnreachableAddress(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return errors.New(staleAddressError) })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"queued follow-up"}`, filepath.Join(dir, "gone.jsonl"))
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("follow-up to an unreachable address: status=%d, want 409", response.StatusCode)
	}
}

func postInboxWake(t *testing.T, baseURL, body, session string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/inbox/items", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(sessionPathHeader, session)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}
