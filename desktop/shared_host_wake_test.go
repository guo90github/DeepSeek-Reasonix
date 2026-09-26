package main

import "testing"

// A wake must never fall back to another tab: it goes to the tab owning the
// session the child named, and nowhere when that tab is gone. Guessing here is
// how one session's mention would arrive in a different session's inbox.
func TestControllerForSessionPathNeverFallsBackToAnotherTab(t *testing.T) {
	app := NewApp()
	app.tabs["a"] = &WorkspaceTab{ID: "a", SessionPath: "/sessions/a.jsonl"}

	if got := app.controllerForSessionPath(""); got != nil {
		t.Fatal("an empty session path resolved a controller")
	}
	if got := app.controllerForSessionPath("/sessions/gone.jsonl"); got != nil {
		t.Fatal("a wake naming another session resolved this tab's controller")
	}
	if got := app.controllerForSessionPath("/sessions/a.jsonl"); got != nil {
		t.Fatal("a tab with no controller resolved one")
	}
}
