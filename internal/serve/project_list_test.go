package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

func projectListServer(t *testing.T, list func() []ProjectEntry) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cur.jsonl")
	saveServeTestSession(t, path)

	bc := NewBroadcaster()
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, bc)
	ctrl := control.New(control.Options{Executor: exec, Sink: bc, SessionDir: dir, SessionPath: path})
	server := New(ctrl, bc, config.ServeConfig{})
	server.SetProjectLister(list)
	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestProjectsNeedsAHost pins the standalone-serve contract: without a host
// there is no workspace list to report, and "no list" must be an explicit 501
// so a remote caller falls back to the projects it can see in /sessions
// instead of rendering an empty picker.
func TestProjectsNeedsAHost(t *testing.T) {
	srv := projectListServer(t, nil)
	resp, err := http.Get(srv.URL + "/projects")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("GET /projects without a lister = %d, want 501", resp.StatusCode)
	}
}

// TestProjectsListsHostWorkspaces covers the list a caller turns into a picker:
// blank roots are dropped rather than offered as an unopenable choice, and the
// host's order is preserved so its first entry stays the default.
func TestProjectsListsHostWorkspaces(t *testing.T) {
	srv := projectListServer(t, func() []ProjectEntry {
		return []ProjectEntry{
			{Root: " /tmp/global ", Name: "global", Scope: "global"},
			{Root: "   "},
			{Root: "/tmp/demo", Name: "demo", Scope: "project"},
		}
	})

	resp, err := http.Get(srv.URL + "/projects")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /projects = %d, want 200", resp.StatusCode)
	}
	var rows []ProjectEntry
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("GET /projects = %+v, want the blank root dropped", rows)
	}
	if rows[0].Root != "/tmp/global" || rows[0].Scope != "global" {
		t.Fatalf("first entry = %+v, want the trimmed global workspace", rows[0])
	}
	if rows[1].Root != "/tmp/demo" || rows[1].Name != "demo" || rows[1].Scope != "project" {
		t.Fatalf("second entry = %+v, want the project root", rows[1])
	}
}
