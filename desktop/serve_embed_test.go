package main

import (
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
)

// The spawn window is the hazard: children spawn while tabs build, the embedded
// listener comes up only once a foreground tab is ready, and plugin.Host
// resolves this provider once per spawn for the child's whole life. Gating the
// env on the listener therefore left the child with no env at all, so a remote
// wake travelled on the client's own defaults and landed in whichever tab was
// foreground instead of the session that was asked for.
func TestHostProcessEnvIsCompleteBeforeListenerStarts(t *testing.T) {
	embeddedServeState.Store(nil) // no listener yet: exactly the spawn window
	t.Cleanup(func() { embeddedServeState.Store(nil) })

	root := filepath.Join(t.TempDir(), "mobile")
	sessionPath := filepath.Join(t.TempDir(), "sessions", "20260922-105039.138936100-x.jsonl")
	tab := &WorkspaceTab{ID: "tab_mobile", WorkspaceRoot: root, SessionPath: sessionPath}
	app := NewApp()
	app.tabs = map[string]*WorkspaceTab{tab.ID: tab}
	app.activeTabID = "tab_other_root" // the foreground tab belongs to another root

	env := app.hostProcessEnvForRoot(root)
	if got := env["REASONIX_SERVE_URL"]; got != "http://127.0.0.1:8787" {
		t.Errorf("REASONIX_SERVE_URL = %q, want the loopback form of %s", got, embeddedServeAddr)
	}
	if want := serveTokenPath(); env["REASONIX_SERVE_TOKEN_FILE"] != want {
		t.Errorf("REASONIX_SERVE_TOKEN_FILE = %q, want %q", env["REASONIX_SERVE_TOKEN_FILE"], want)
	}
	want := agent.CanonicalSessionPath(sessionPath)
	if got := env["REASONIX_SESSION_PATH"]; got != want {
		t.Errorf("REASONIX_SESSION_PATH = %q, want %q: without it a wake addresses the foreground tab", got, want)
	}
}

// Global tabs carry globalSharedHostKey rather than their empty WorkspaceRoot,
// and a root's own tabs are searched in tabOrder. Both decide which session a
// remote wake addresses, so neither may depend on a map iteration order or on a
// key that can never match.
func TestSessionPathForRootResolvesGlobalKeyAndTabOrder(t *testing.T) {
	app := NewApp()
	global := &WorkspaceTab{ID: "tab_global", SessionPath: filepath.Join("sessions", "global.jsonl")}
	first := &WorkspaceTab{ID: "tab_a1", WorkspaceRoot: `C:\ws\a`, SessionPath: filepath.Join("sessions", "a1.jsonl")}
	second := &WorkspaceTab{ID: "tab_a2", WorkspaceRoot: `C:\ws\a`, SessionPath: filepath.Join("sessions", "a2.jsonl")}
	other := &WorkspaceTab{ID: "tab_b", WorkspaceRoot: `C:\ws\b`, SessionPath: filepath.Join("sessions", "b.jsonl")}
	app.tabs = map[string]*WorkspaceTab{global.ID: global, first.ID: first, second.ID: second, other.ID: other}
	app.tabOrder = []string{global.ID, first.ID, second.ID, other.ID}
	app.activeTabID = other.ID

	if got := app.sessionPathForRoot(globalSharedHostKey); got != agent.CanonicalSessionPath(global.SessionPath) {
		t.Errorf("global key = %q, want the global tab's session %q", got, global.SessionPath)
	}
	// No active tab inside the root: the newest tab the window holds for it wins,
	// and the answer must not move between calls.
	want := agent.CanonicalSessionPath(second.SessionPath)
	for i := 0; i < 4; i++ {
		if got := app.sessionPathForRoot(`C:\ws\a`); got != want {
			t.Fatalf("root without an active tab = %q, want %q", got, want)
		}
	}
	app.activeTabID = first.ID
	if got := app.sessionPathForRoot(`C:\ws\a`); got != agent.CanonicalSessionPath(first.SessionPath) {
		t.Errorf("active tab of the root = %q, want %q", got, first.SessionPath)
	}
	if got := app.sessionPathForRoot(`C:\ws\nowhere`); got != "" {
		t.Errorf("unknown root = %q, want empty", got)
	}
}

// Once the listener is up its real port wins, so a window that moved off 8787
// still hands its children a dialable address.
func TestEmbeddedServeURLUsesListenerPort(t *testing.T) {
	if got := embeddedServeURL(&embeddedServe{addr: "0.0.0.0:9123"}); got != "http://127.0.0.1:9123" {
		t.Errorf("embeddedServeURL = %q, want http://127.0.0.1:9123", got)
	}
	if got := embeddedServeURL(nil); got != "http://127.0.0.1:8787" {
		t.Errorf("embeddedServeURL(nil) = %q, want http://127.0.0.1:8787", got)
	}
}
