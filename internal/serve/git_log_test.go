package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

func gitLogServer(t *testing.T, read func(root string) ([]GitCommit, error)) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cur.jsonl")
	saveServeTestSession(t, path)

	bc := NewBroadcaster()
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, bc)
	ctrl := control.New(control.Options{Executor: exec, Sink: bc, SessionDir: dir, SessionPath: path})
	server := New(ctrl, bc, config.ServeConfig{})
	server.SetGitLogReader(read)
	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestGitLogNeedsAHost pins the standalone-serve contract: a remote client has
// no filesystem of its own, so with no host git reader there is nothing to read
// and the answer must be an explicit 501 — never an empty history, which the
// phone would render as "this workspace has no commits".
func TestGitLogNeedsAHost(t *testing.T) {
	srv := gitLogServer(t, nil)
	resp, err := http.Get(srv.URL + "/git-log")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("GET /git-log without a reader = %d, want 501", resp.StatusCode)
	}
}

// TestGitLogForwardsRequestedRoot covers the root the caller names: it must
// reach the host unchanged (trimmed), because the host validates it against the
// workspaces it offered, and an empty root is the "my foreground workspace"
// form the phone sends when it names no project.
func TestGitLogForwardsRequestedRoot(t *testing.T) {
	var seen []string
	srv := gitLogServer(t, func(root string) ([]GitCommit, error) {
		seen = append(seen, root)
		return []GitCommit{}, nil
	})

	for _, tc := range []struct{ query, want string }{
		{"?root=%20%2Ftmp%2Fdemo%20", "/tmp/demo"},
		{"", ""},
	} {
		resp, err := http.Get(srv.URL + "/git-log" + tc.query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /git-log%s = %d, want 200", tc.query, resp.StatusCode)
		}
	}
	if len(seen) != 2 || seen[0] != "/tmp/demo" || seen[1] != "" {
		t.Fatalf("host saw roots %q, want [/tmp/demo ]", seen)
	}
}

// TestGitLogReadsAsAnArrayNotJSONNull covers the empty case the phone switches
// on: a workspace with no commits (or a host that answers nil) must arrive as
// [] so the list renders empty instead of the client tripping over null.
func TestGitLogReadsAsAnArrayNotJSONNull(t *testing.T) {
	srv := gitLogServer(t, func(string) ([]GitCommit, error) { return nil, nil })

	resp, err := http.Get(srv.URL + "/git-log")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /git-log = %d, want 200", resp.StatusCode)
	}
	var raw any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if rows, ok := raw.([]any); !ok || len(rows) != 0 {
		t.Fatalf("GET /git-log body = %#v, want an empty array", raw)
	}
}

// TestGitLogReportsCommitFields covers the four fields the phone renders. A
// missing one is silent on screen (an empty author line), so they are pinned.
func TestGitLogReportsCommitFields(t *testing.T) {
	srv := gitLogServer(t, func(string) ([]GitCommit, error) {
		return []GitCommit{{Hash: "abc123", Author: "guosj", Date: "2026-10-05", Message: "修一个问题"}}, nil
	})

	resp, err := http.Get(srv.URL + "/git-log")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rows []GitCommit
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("GET /git-log = %+v, want one commit", rows)
	}
	want := GitCommit{Hash: "abc123", Author: "guosj", Date: "2026-10-05", Message: "修一个问题"}
	if rows[0] != want {
		t.Fatalf("commit = %+v, want %+v", rows[0], want)
	}
}

// TestGitLogSurfacesReadFailure covers an unreadable workspace (not a git repo,
// or git missing): the reason must reach the caller so the phone can say why
// rather than showing a plausible-looking empty history.
func TestGitLogSurfacesReadFailure(t *testing.T) {
	srv := gitLogServer(t, func(string) ([]GitCommit, error) {
		return nil, errors.New("exit status 128: not a git repository")
	})

	resp, err := http.Get(srv.URL + "/git-log")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("GET /git-log with a failing reader = %d, want 502", resp.StatusCode)
	}
}
