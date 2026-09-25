package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/sessioninbox"
)

// A sender once had exactly one way to tell the three 409 sources apart: read
// the prose. The class header is the contract that replaces it (measured
// 2026-09-25 against a real chatting room, where a retry loop could not tell a
// mis-addressed wake from a full queue).
func TestInboxRefusalCarriesTheCallersRetryClass(t *testing.T) {
	dir := t.TempDir()
	runner := runtimeStateServeRunner{started: make(chan struct{})}
	foreground := runtimeStateServeController(t, dir, "foreground", runner)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return errors.New(unreachableAddressError) })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, filepath.Join(dir, "gone.jsonl"))
	assertRejectClass(t, response, rejectTargetUnreachable, false)
}

func TestInboxRefusalClassifiesAnUnhonoredAddress(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	server.SetSessionActivator(func(string) error { return nil })
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, filepath.Join(dir, "elsewhere.jsonl"))
	assertRejectClass(t, response, rejectTargetUnreachable, false)
}

func TestInboxRefusalClassifiesAStaleExpectedSession(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/inbox/items", strings.NewReader(`{"input":"room wake"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(expectedSessionPathHeader, filepath.Join(dir, "other.jsonl"))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	assertRejectClass(t, response, rejectTargetUnreachable, false)
}

func TestInboxRefusalClassifiesAMissingItemAsUnretryable(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodPatch, httpServer.URL+"/inbox/items/missing", strings.NewReader(`{"input":"edited"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	assertRejectClass(t, response, rejectInvalidRequest, false)
}

// Capacity and pause clear on their own, so the host must ask for a wait
// instead of reporting a refusal with the same authority as a wrong address.
func TestInboxTransientRefusalsAskTheCallerToWait(t *testing.T) {
	for _, err := range []error{sessioninbox.ErrCapacityItems, sessioninbox.ErrCapacityBytes, sessioninbox.ErrPaused} {
		recorder := httptest.NewRecorder()
		writeInboxError(recorder, err)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("%v: status=%d, want 409", err, recorder.Code)
		}
		assertRejectClass(t, recorder.Result(), rejectNotAccepting, true)
	}
}

func TestInboxUnretryableRefusalsCarryNoWait(t *testing.T) {
	for _, err := range []error{sessioninbox.ErrInvalidState, sessioninbox.ErrNotFound, sessioninbox.ErrIdempotencyConflict} {
		recorder := httptest.NewRecorder()
		writeInboxError(recorder, err)
		assertRejectClass(t, recorder.Result(), rejectInvalidRequest, false)
	}
}

// The class is the inbox contract, not every failure of the endpoint: a body
// the caller can fix needs no retry policy.
func TestInboxUnclassifiedFailuresStayUnclassified(t *testing.T) {
	for _, want := range []struct {
		err    error
		status int
	}{
		{sessioninbox.ErrItemTooLarge, http.StatusRequestEntityTooLarge},
		{sessioninbox.ErrEmpty, http.StatusBadRequest},
	} {
		recorder := httptest.NewRecorder()
		writeInboxError(recorder, want.err)
		if recorder.Code != want.status {
			t.Fatalf("%v: status=%d, want %d", want.err, recorder.Code, want.status)
		}
		if class := recorder.Header().Get(rejectClassHeader); class != "" {
			t.Fatalf("%v: class=%q, want none", want.err, class)
		}
	}
}

func assertRejectClass(t *testing.T, response *http.Response, want string, wantWait bool) {
	t.Helper()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", response.StatusCode)
	}
	if class := response.Header.Get(rejectClassHeader); class != want {
		t.Fatalf("class=%q, want %q", class, want)
	}
	if wait := response.Header.Get("Retry-After"); (wait != "") != wantWait {
		t.Fatalf("Retry-After=%q, want wait=%t", wait, wantWait)
	}
}
