package serve

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

func hostCreatorServer(t *testing.T, create func(context.Context, NewSessionRequest) (string, error)) (*httptest.Server, *int) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cur.jsonl")
	saveServeTestSession(t, path)

	bc := NewBroadcaster()
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, bc)
	ctrl := control.New(control.Options{Executor: exec, Sink: bc, SessionDir: dir, SessionPath: path})
	server := New(ctrl, bc, config.ServeConfig{})

	calls := 0
	server.SetSessionCreator(func(ctx context.Context, req NewSessionRequest) (string, error) {
		calls++
		return create(ctx, req)
	})
	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)
	return srv, &calls
}

func postNew(t *testing.T, srv *httptest.Server, expected string) (*http.Response, string) {
	t.Helper()
	return postNewBody(t, srv, expected, `{}`)
}

func postNewBody(t *testing.T, srv *httptest.Server, expected, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/new", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if expected != "" {
		req.Header.Set(expectedSessionPathHeader, expected)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := readAll(resp)
	return resp, text
}

// TestNewSessionDelegatesToHost covers the embedded-host contract: the host
// decides what "new session" means (it opens its own blank surface), and a
// stale expected-session pin must not refuse the request — the pin guards
// rotation of a displayed session, and opening a new one misroutes nothing.
func TestNewSessionDelegatesToHost(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "host.jsonl")
	srv, calls := hostCreatorServer(t, func(context.Context, NewSessionRequest) (string, error) {
		return hostPath, nil
	})

	resp, body := postNew(t, srv, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("/new status = %d body %q, want 204", resp.StatusCode, body)
	}
	if got, want := resp.Header.Get(sessionPathHeader), agent.CanonicalSessionPath(hostPath); got != want {
		t.Fatalf("%s = %q, want %q", sessionPathHeader, got, want)
	}

	stale, staleBody := postNew(t, srv, filepath.Join(t.TempDir(), "elsewhere.jsonl"))
	if stale.StatusCode != http.StatusNoContent {
		t.Fatalf("/new with stale pin status = %d body %q, want 204", stale.StatusCode, staleBody)
	}
	if *calls != 2 {
		t.Fatalf("host creator calls = %d, want 2", *calls)
	}
}

// TestNewSessionHostFailureIsConflict pins the failure contract the remote
// client renders: a host that cannot open a session answers 409 with its own
// reason, and one that opens nothing is refused instead of announcing a blank.
func TestNewSessionHostFailureIsConflict(t *testing.T) {
	cases := []struct {
		name   string
		create func(context.Context, NewSessionRequest) (string, error)
		want   string
	}{
		{"host error", func(context.Context, NewSessionRequest) (string, error) { return "", errors.New("no window") }, "no window"},
		{"empty path", func(context.Context, NewSessionRequest) (string, error) { return "  ", nil }, "host opened no session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := hostCreatorServer(t, tc.create)
			resp, body := postNew(t, srv, "")
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("/new status = %d body %q, want 409", resp.StatusCode, body)
			}
			if body != tc.want {
				t.Fatalf("/new body = %q, want %q", body, tc.want)
			}
		})
	}
}

// TestNewSessionCarriesTargetToHost pins the remote-binding contract: a caller
// that names a workspace gets that workspace — the host resolves the scope — so
// the target must survive the wire, a malformed one must be refused rather than
// silently demoted to the window's default, and no target at all must still
// reach the host as the zero request.
func TestNewSessionCarriesTargetToHost(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "host.jsonl")
	var got NewSessionRequest
	srv, _ := hostCreatorServer(t, func(_ context.Context, req NewSessionRequest) (string, error) {
		got = req
		return hostPath, nil
	})

	resp, body := postNewBody(t, srv, "", `{"scope":"project","projectRoot":" /tmp/demo "}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("/new with a target status = %d body %q, want 204", resp.StatusCode, body)
	}
	if got.Scope != "project" || got.ProjectRoot != "/tmp/demo" {
		t.Fatalf("host request = %+v, want the trimmed project target", got)
	}

	bad, badBody := postNewBody(t, srv, "", `{"scope":"project"}`)
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("/new without a root = %d body %q, want 400", bad.StatusCode, badBody)
	}
	if unknown, unknownBody := postNewBody(t, srv, "", `{"scope":"workspace"}`); unknown.StatusCode != http.StatusBadRequest {
		t.Fatalf("/new with an unknown scope = %d body %q, want 400", unknown.StatusCode, unknownBody)
	}

	if resp, body := postNewBody(t, srv, "", ``); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("/new with no body = %d body %q, want 204", resp.StatusCode, body)
	}
	if got.Scope != "" || got.ProjectRoot != "" {
		t.Fatalf("a bodyless /new reached the host as %+v, want the zero target", got)
	}
}
