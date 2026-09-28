package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/serve"
)

const serveSessionPathHeader = "X-Reasonix-Session-Path"

// The phone's binding path end to end, over the real HTTP surface and the real
// host wiring: GET /projects offers the desktop's workspaces, and POST /new
// with a named project opens the session there instead of in whatever tab the
// window happens to show. Component tests cover each half; this one covers the
// wiring between them, where an unregistered hook looks like a working feature.
func TestRemoteNewSessionBindsRequestedProjectOverHTTP(t *testing.T) {
	isolateDesktopUserDirs(t)
	foregroundRoot := t.TempDir()
	wantedRoot := t.TempDir()
	rememberWorkspace(wantedRoot)

	app := NewApp()
	tab, err := app.EnsureBlankTab("project", foregroundRoot)
	if err != nil {
		t.Fatal(err)
	}
	app.activeTabID = tab.ID

	seedPath := filepath.Join(foregroundRoot, "sessions", "seed.jsonl")
	bc := serve.NewBroadcaster()
	srv := serve.New(controllerWithContent(t, seedPath), bc, config.ServeConfig{})
	app.attachEmbeddedServeHooks(srv)
	httpSrv := httptest.NewServer(srv.Handler())
	t.Cleanup(httpSrv.Close)

	projects := getProjects(t, httpSrv.URL)
	if len(projects) == 0 || projects[0].Scope != "global" {
		t.Fatalf("GET /projects = %+v, want the global scope first", projects)
	}
	var offered bool
	for _, project := range projects {
		if sameProjectRoot(project.Root, wantedRoot) {
			offered = project.Scope == "project"
		}
	}
	if !offered {
		t.Fatalf("GET /projects = %+v, want the remembered workspace as a project", projects)
	}

	body, err := json.Marshal(serve.NewSessionRequest{Scope: "project", ProjectRoot: wantedRoot})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(httpSrv.URL+"/new", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /new = %d body %q, want 204", resp.StatusCode, payload)
	}
	path := resp.Header.Get(serveSessionPathHeader)
	if path == "" {
		t.Fatal("POST /new returned no session path")
	}
	active, _ := app.activeTabAndCtrl()
	if active == nil {
		t.Fatal("no active tab after the remote new-session")
	}
	if !sameProjectRoot(active.WorkspaceRoot, wantedRoot) {
		t.Fatalf("workspace = %q, want the requested %q (foreground was %q)",
			active.WorkspaceRoot, wantedRoot, foregroundRoot)
	}
	if !sameProjectRoot(active.SessionPath, path) {
		t.Fatalf("returned %q, but the foreground tab owns %q", path, active.SessionPath)
	}
}

func getProjects(t *testing.T, base string) []serve.ProjectEntry {
	t.Helper()
	resp, err := http.Get(base + "/projects")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /projects = %d, want 200", resp.StatusCode)
	}
	var rows []serve.ProjectEntry
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}
