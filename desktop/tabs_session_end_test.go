package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Closing a tab must hand the ended session's path to the end seam: derived
// projections (session recaps) name the session from it, and the desktop used to
// clear the path before Close, which delivered an empty path instead.
func TestCloseTabReportsEndedSessionPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")

	// Built directly so the concrete controller (and its end seam) is reachable.
	ctrl := controllerWithContent(t, path)
	tab := &WorkspaceTab{
		ID:            "test_tab",
		Ctrl:          ctrl,
		Scope:         "global",
		WorkspaceRoot: "",
		Ready:         true,
		disabledMCP:   map[string]ServerView{},
	}
	app := &App{
		tabs:        map[string]*WorkspaceTab{"test_tab": tab},
		activeTabID: "test_tab",
	}
	tab.sink = &tabEventSink{tabID: tab.ID, app: app}

	// CloseTab needs more than one tab and a tab left active.
	survivor := &WorkspaceTab{
		ID:          "survivor_tab",
		Scope:       "global",
		Ready:       true,
		disabledMCP: map[string]ServerView{},
	}
	survivor.sink = &tabEventSink{tabID: survivor.ID, app: app}
	app.tabs["survivor_tab"] = survivor

	// Only a controller that ran a turn reports a session end.
	if err := ctrl.Run(context.Background(), "close me"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ended := make(chan string, 4)
	ctrl.SetSessionEndObserver(func(_, sessionPath string) { ended <- sessionPath })

	if err := app.CloseTab("test_tab"); err != nil {
		t.Fatalf("CloseTab: %v", err)
	}

	select {
	case got := <-ended:
		if got != path {
			t.Fatalf("end seam saw %q, want %q", got, path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing a tab never reported the ended session to the end seam")
	}
}
