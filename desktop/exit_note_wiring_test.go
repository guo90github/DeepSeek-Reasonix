package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostStatePathForTest points the marker at a temp file: the production path is
// the running app's own marker, and a test must never clear that.
func hostStatePathForTest(t *testing.T) {
	t.Helper()
	previous := hostStatePathFunc
	t.Cleanup(func() { hostStatePathFunc = previous })
	// Resolve the directory once: a closure that called t.TempDir() would hand the
	// write and the read two different files.
	path := filepath.Join(t.TempDir(), "host-state.json")
	hostStatePathFunc = func() string { return path }
}

// The shutdown path is the one place a deliberate quit is knowable. Without the
// note a clean exit and a kill read identically to the next launch.
func TestCleanShutdownWritesACleanExitNote(t *testing.T) {
	exitNoteDirForTest(t)
	hostStatePathForTest(t)
	tracker := lifecycleTrackerForTest(t, t.TempDir(), os.Getpid(), "clean-exit")
	noteDesktopRunStarted()

	completeDesktopShutdown(tracker, func() {})

	note, ok := readDesktopExitNote(desktopRunID)
	if !ok {
		t.Fatal("the clean shutdown left no note")
	}
	if note.Kind != exitKindClean || note.Phase != "shutting_down" || note.ExitedAt == "" {
		t.Fatalf("clean exit note = %+v, want the clean kind with an exit time", note)
	}
}

// A panic that unwinds to the process edge is the crash the next launch has to
// explain, so the note must be on disk before the re-panic. The path itself
// cannot run in a test without writing a real crash report into the user's state
// directory, so the ordering is pinned at the source.
func TestRecoveredPanicWritesTheNoteBeforeRepanicking(t *testing.T) {
	source, err := os.ReadFile("crash_pending.go")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(source), "func (a *App) recoverToPending(site string) {")
	if !ok {
		t.Fatal("crash_pending.go no longer declares recoverToPending")
	}
	noteAt := strings.Index(body, "noteDesktopRunExited(exitKindPanic")
	repanicAt := strings.Index(body, "panic(r)")
	if noteAt < 0 {
		t.Fatal("recoverToPending must record a panic exit note")
	}
	if repanicAt < 0 || noteAt > repanicAt {
		t.Fatal("the panic note must be written before the process dies again")
	}
}

// One run must read as one run: the exit note, the observer and the marker the
// watchdog reads all name the same id.
func TestHostLaunchAndExitNoteShareOneRunID(t *testing.T) {
	exitNoteDirForTest(t)
	hostStatePathForTest(t)

	noteDesktopRunStarted()
	noteHostLaunch(false)

	note, ok := readDesktopExitNote(desktopRunID)
	if !ok || note.RunID != desktopRunID {
		t.Fatalf("exit note = %+v (%v), want this run's id", note, ok)
	}
	record, ok := readHostState()
	if !ok {
		t.Fatal("the launch wrote no host-state marker")
	}
	if record.RunID != desktopRunID {
		t.Fatalf("marker runId = %q, want %q", record.RunID, desktopRunID)
	}
}
