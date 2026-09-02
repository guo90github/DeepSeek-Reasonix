package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/provider"
)

func TestHistorySliceColdPathBeforeControllerReady(t *testing.T) {
	isolateDesktopUserDirs(t)
	root := t.TempDir()
	dir := desktopSessionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	messages := make([]provider.Message, 0, 80)
	for i := range 40 {
		messages = append(messages,
			provider.Message{Role: provider.RoleUser, Content: fmt.Sprintf("cold user turn %d large-session marker", i)},
			provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("cold assistant turn %d large-session marker", i)},
		)
	}
	_, path := saveHistorySliceSession(t, dir, "cold-large.jsonl", messages)
	app := NewApp()
	tab := &WorkspaceTab{
		ID:            "cold-large",
		Scope:         "project",
		WorkspaceRoot: root,
		SessionPath:   path,
		Ready:         false,
		Ctrl:          nil,
	}
	app.mu.Lock()
	app.tabs = map[string]*WorkspaceTab{tab.ID: tab}
	app.activeTabID = tab.ID
	app.mu.Unlock()

	page := app.HistorySliceForTab(tab.ID, HistorySliceRequest{Turns: 12})
	if page.Error != "" {
		t.Fatalf("cold slice error = %q", page.Error)
	}
	if len(page.Entries) == 0 {
		t.Fatal("cold slice returned no entries before controller ready")
	}
	if page.Source != "index" && page.Source != "scan" {
		t.Fatalf("cold source = %q, want index|scan", page.Source)
	}
}

func TestHistorySliceReportsErrorInsteadOfEmptySuccess(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	tab := &WorkspaceTab{
		ID:          "missing",
		Scope:       "project",
		SessionPath: filepath.Join(t.TempDir(), "does-not-exist.jsonl"),
		Ready:       false,
	}
	app.mu.Lock()
	app.tabs = map[string]*WorkspaceTab{tab.ID: tab}
	app.activeTabID = tab.ID
	app.mu.Unlock()

	page := app.HistorySliceForTab(tab.ID, HistorySliceRequest{})
	if page.Error == "" {
		t.Fatal("missing session should report Error, not a silent empty success")
	}
	if len(page.Entries) != 0 {
		t.Fatalf("entries = %d, want 0 on error", len(page.Entries))
	}
}

// TestHistorySliceWaitsForInFlightBuild guards the first-open-after-restart
// race for cursor paging: HistorySliceForTab with a cursor must block on the
// tab's background controller build instead of returning the not-yet-ready
// error, then serve once the build settles. A first-page request over an
// already-known session file skips this wait (it is served cold immediately).
func TestHistorySliceWaitsForInFlightBuild(t *testing.T) {
	isolateDesktopUserDirs(t)
	root := t.TempDir()
	dir := desktopSessionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	messages := make([]provider.Message, 0, 40)
	for i := range 20 {
		messages = append(messages,
			provider.Message{Role: provider.RoleUser, Content: fmt.Sprintf("wait user %d", i)},
			provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("wait assistant %d", i)},
		)
	}
	_, path := saveHistorySliceSession(t, dir, "wait-build.jsonl", messages)
	app := NewApp()
	buildDone := make(chan struct{})
	tab := &WorkspaceTab{
		ID:            "wait-build",
		Scope:         "project",
		WorkspaceRoot: root,
		SessionPath:   path,
		Ready:         false,
		Ctrl:          nil,
		buildDone:     buildDone,
		buildDoneGen:  1,
	}
	app.mu.Lock()
	app.tabs = map[string]*WorkspaceTab{tab.ID: tab}
	app.activeTabID = tab.ID
	app.mu.Unlock()

	// The first page over the known session file is served cold without
	// waiting for the in-flight build.
	first := app.HistorySliceForTab(tab.ID, HistorySliceRequest{Turns: 12})
	if first.Error != "" || len(first.Entries) == 0 {
		t.Fatalf("first page before build: error=%q entries=%d", first.Error, len(first.Entries))
	}
	if first.NextCursor == "" {
		t.Fatal("first page has no cursor to page with")
	}

	// A cursor request must wait for the in-flight build.
	done := make(chan HistorySlice, 1)
	go func() {
		done <- app.HistorySliceForTab(tab.ID, HistorySliceRequest{Turns: 12, Cursor: first.NextCursor})
	}()
	select {
	case <-done:
		t.Fatal("HistorySliceForTab with a cursor returned before the in-flight build completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(buildDone)
	select {
	case page := <-done:
		if page.Error != "" {
			t.Fatalf("slice error after build settled = %q", page.Error)
		}
		if len(page.Entries) == 0 {
			t.Fatal("no entries after build settled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HistorySliceForTab did not resume after the build completed")
	}
}

// TestHistorySliceDoesNotWaitForReadyTab verifies a ready tab short-circuits
// the build wait (no channel, no timeout) so the live path stays cheap.
func TestHistorySliceDoesNotWaitForReadyTab(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	tab := &WorkspaceTab{ID: "ready", Ready: true}
	app.mu.Lock()
	app.tabs = map[string]*WorkspaceTab{tab.ID: tab}
	app.activeTabID = tab.ID
	app.mu.Unlock()
	// A ready tab with no controller/session must not block; it returns the
	// not-ready failure instead of hanging on an empty build.
	page := app.HistorySliceForTab(tab.ID, HistorySliceRequest{})
	if page.Error == "" {
		t.Fatal("ready-but-unbound tab should report an error, not hang")
	}
}
