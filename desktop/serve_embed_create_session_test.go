package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/serve"
)

// The phone's /new must become the window's own move: a blank surface for the
// active tab's scope. A remote "new session" that landed in another workspace
// would hand the phone a session the user is not looking at.
func TestCreateSessionForRemoteFollowsActiveTabScope(t *testing.T) {
	isolateDesktopUserDirs(t)
	projectRoot := t.TempDir()

	app := NewApp()
	for _, tc := range []struct {
		name      string
		scope     string
		root      string
		wantScope string
	}{
		{"global foreground", "global", "", "global"},
		{"project foreground", "project", projectRoot, "project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tab, err := app.EnsureBlankTab(tc.scope, tc.root)
			if err != nil {
				t.Fatal(err)
			}
			app.activeTabID = tab.ID

			path, err := app.createSessionForRemote(t.Context(), serve.NewSessionRequest{})
			if err != nil {
				t.Fatalf("createSessionForRemote: %v", err)
			}
			active, _ := app.activeTabAndCtrl()
			if active == nil {
				t.Fatal("no active tab after the remote new-session")
			}
			if active.Scope != tc.wantScope {
				t.Errorf("scope = %q, want %q", active.Scope, tc.wantScope)
			}
			if agent.CanonicalSessionPath(active.SessionPath) != agent.CanonicalSessionPath(path) {
				t.Errorf("returned %q, but the foreground tab owns %q", path, active.SessionPath)
			}
		})
	}
}

// A caller that names a workspace gets that workspace, whatever the window's
// foreground tab happens to be: the phone picked a project and the desktop must
// not decide for it. A malformed target is refused instead of silently falling
// back to the foreground project, which is how a remote session ends up bound
// to a workspace the user never chose.
func TestCreateSessionForRemoteBindsRequestedProject(t *testing.T) {
	isolateDesktopUserDirs(t)
	foregroundRoot := t.TempDir()
	wantedRoot := t.TempDir()

	app := NewApp()
	tab, err := app.EnsureBlankTab("project", foregroundRoot)
	if err != nil {
		t.Fatal(err)
	}
	app.activeTabID = tab.ID

	path, err := app.createSessionForRemote(t.Context(), serve.NewSessionRequest{Scope: "project", ProjectRoot: wantedRoot})
	if err != nil {
		t.Fatalf("createSessionForRemote: %v", err)
	}
	active, _ := app.activeTabAndCtrl()
	if active == nil {
		t.Fatal("no active tab after the targeted new-session")
	}
	if !sameProjectRoot(active.WorkspaceRoot, wantedRoot) {
		t.Fatalf("workspace = %q, want the requested %q", active.WorkspaceRoot, wantedRoot)
	}
	if agent.CanonicalSessionPath(active.SessionPath) != agent.CanonicalSessionPath(path) {
		t.Fatalf("returned %q, but the foreground tab owns %q", path, active.SessionPath)
	}

	if _, err := app.createSessionForRemote(t.Context(), serve.NewSessionRequest{Scope: "global"}); err != nil {
		t.Fatalf("global target: %v", err)
	}
	if active, _ := app.activeTabAndCtrl(); active == nil || active.Scope != "global" {
		t.Fatalf("global target landed in %+v, want the global scope", active)
	}

	for _, req := range []serve.NewSessionRequest{
		{Scope: "project"},
		{Scope: "workspace", ProjectRoot: wantedRoot},
		{Scope: "project", ProjectRoot: filepath.Join(wantedRoot, "missing")},
	} {
		if _, err := app.createSessionForRemote(t.Context(), req); err == nil {
			t.Fatalf("%+v was accepted, want a refusal", req)
		}
	}
}

// The remote "new session" follows the window's layout exactly as the app
// shell's own does: the single-surface styles replace the one surface they
// show, while Split adds a surface and leaves the open conversations alone.
func TestCreateSessionForRemoteFollowsLayoutStyle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		style    string
		wantTabs int
	}{
		{"workbench replaces the visible surface", "workbench", 1},
		{"split keeps the open conversations", "split", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateDesktopUserDirs(t)
			if err := editUserConfig(func(c *config.Config) error {
				return c.SetDesktopLayoutStyle(tc.style)
			}); err != nil {
				t.Fatal(err)
			}

			app := NewApp()
			root := t.TempDir()
			path := filepath.Join(root, "sessions", "kept.jsonl")
			ctrl := controllerWithContent(t, path)
			if err := ctrl.Snapshot(); err != nil {
				t.Fatalf("snapshot the kept session: %v", err)
			}
			kept := &WorkspaceTab{
				ID: "kept", Scope: "project", WorkspaceRoot: root, SessionPath: path,
				disabledMCP: map[string]ServerView{},
			}
			app.tabs = map[string]*WorkspaceTab{kept.ID: kept}
			app.tabOrder = []string{kept.ID}
			app.activeTabID = kept.ID

			newPath, err := app.createSessionForRemote(t.Context(), serve.NewSessionRequest{})
			if err != nil {
				t.Fatalf("createSessionForRemote: %v", err)
			}
			if got := len(app.tabs); got != tc.wantTabs {
				_, keptAlive := app.tabs[kept.ID]
				active, _ := app.activeTabAndCtrl()
				id := ""
				if active != nil {
					id = active.ID
				}
				t.Fatalf("open tabs = %d, want %d (kept alive = %v, active = %q, singleSurface = %v)",
					got, tc.wantTabs, keptAlive, id, app.singleSurfaceLayoutEnabled())
			}
			active, _ := app.activeTabAndCtrl()
			if active == nil || agent.CanonicalSessionPath(active.SessionPath) != agent.CanonicalSessionPath(newPath) {
				t.Fatalf("foreground = %+v, want the new session %q", active, newPath)
			}
			if tc.wantTabs == 2 && app.tabs[kept.ID] != kept {
				t.Error("the remote new-session closed the conversation the user had open")
			}
		})
	}
}

// Refusals must be explicit: a read-only foreground has no writable session to
// open, and a caller that already hung up must not leave a fresh tab behind.
func TestCreateSessionForRemoteRefusesUnwritableForeground(t *testing.T) {
	isolateDesktopUserDirs(t)

	app := NewApp()
	ready, err := app.EnsureBlankTab("global", "")
	if err != nil {
		t.Fatal(err)
	}

	readOnly := &WorkspaceTab{
		ID: "tab_readonly", Scope: "global", ReadOnly: true,
		SessionPath: filepath.Join(t.TempDir(), "browsed.jsonl"),
	}
	app.tabs[readOnly.ID] = readOnly
	app.tabOrder = append(app.tabOrder, readOnly.ID)

	app.activeTabID = readOnly.ID
	_, err = app.createSessionForRemote(t.Context(), serve.NewSessionRequest{})
	if err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("read-only foreground err = %v, want the writable-tab refusal", err)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	app.activeTabID = ready.ID
	if _, err := app.createSessionForRemote(cancelled, serve.NewSessionRequest{}); err == nil {
		t.Error("cancelled request opened a session")
	}
}
