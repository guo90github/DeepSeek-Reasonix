package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func exitNoteDirForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restore, restoreNow := desktopExitNoteDirFunc, desktopExitNoteNow
	t.Cleanup(func() { desktopExitNoteDirFunc, desktopExitNoteNow = restore, restoreNow })
	desktopExitNoteDirFunc = func() string { return dir }
	return dir
}

// The whole point of the note is that a run which never explained itself is
// distinguishable from one that did.
func TestDesktopExitNoteRecordsStartAndExplicitExit(t *testing.T) {
	exitNoteDirForTest(t)

	noteDesktopRunStarted()
	note, ok := readDesktopExitNote(desktopRunID)
	if !ok {
		t.Fatal("the launch note was not readable")
	}
	if note.Kind != exitKindRunning || note.ExitedAt != "" {
		t.Fatalf("launch note = %+v, want a running kind and no exit time", note)
	}
	if note.PID != os.Getpid() || note.RunID != desktopRunID {
		t.Fatalf("launch note does not name this run: %+v", note)
	}

	noteDesktopRunExited(exitKindClean, "window closed", "shutting_down")
	after, ok := readDesktopExitNote(desktopRunID)
	if !ok {
		t.Fatal("the exit note was not readable")
	}
	if after.Kind != exitKindClean || after.Reason != "window closed" || after.Phase != "shutting_down" {
		t.Fatalf("exit note = %+v, want the clean kind with its reason", after)
	}
	if after.ExitedAt == "" {
		t.Fatalf("exit note carries no exit time: %+v", after)
	}
	if after.StartedAt != note.StartedAt {
		t.Fatalf("the exit note lost the launch time: %q vs %q", after.StartedAt, note.StartedAt)
	}
}

// A kill lands between the two writes, so both kinds of damage are normal: a
// note file that is corrupt, and an exit recorded with no launch note behind it.
func TestDesktopExitNoteSurvivesDamageAndMissingLaunchNote(t *testing.T) {
	dir := exitNoteDirForTest(t)

	path := filepath.Join(dir, desktopRunID+".json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readDesktopExitNote(desktopRunID); ok {
		t.Fatal("a corrupt note must not read as a note")
	}
	noteDesktopRunExited(exitKindPanic, "recovered from a panic at startup", "")
	repaired, ok := readDesktopExitNote(desktopRunID)
	if !ok || repaired.Kind != exitKindPanic {
		t.Fatalf("an exit after damage must still be recorded: %+v (%v)", repaired, ok)
	}
	if repaired.StartedAt == "" {
		t.Fatalf("the repaired note has no start time: %+v", repaired)
	}
}

func TestLatestDesktopExitNoteSkipsThisRunAndPicksTheNewest(t *testing.T) {
	dir := exitNoteDirForTest(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	desktopExitNoteNow = func() time.Time { return now }

	older := desktopExitNote{RunID: "aaaa1111", PID: 4242, Kind: exitKindClean,
		StartedAt: now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
		ExitedAt:  now.Add(-time.Hour).Format(time.RFC3339Nano)}
	newer := desktopExitNote{RunID: "bbbb2222", PID: 4343, Kind: exitKindRunning,
		StartedAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano)}
	for _, entry := range []struct {
		note     desktopExitNote
		modified time.Time
	}{
		{note: older, modified: now.Add(-time.Hour)},
		{note: newer, modified: now.Add(-30 * time.Minute)},
	} {
		if !writeDesktopExitNote(entry.note) {
			t.Fatalf("could not write note %s", entry.note.RunID)
		}
		// A note without an exit time is ordered by file time, so the test has to
		// place the file where the clock seam claims it is.
		path := filepath.Join(dir, entry.note.RunID+".json")
		if err := os.Chtimes(path, entry.modified, entry.modified); err != nil {
			t.Fatal(err)
		}
	}
	noteDesktopRunStarted()

	latest, ok := latestDesktopExitNote(desktopRunID)
	if !ok {
		t.Fatal("no predecessor note found")
	}
	if latest.RunID != "bbbb2222" {
		t.Fatalf("latest = %s, want the newest predecessor (bbbb2222)", latest.RunID)
	}
	if latest.Kind != exitKindRunning || latest.ExitedAt != "" {
		t.Fatalf("a predecessor that never explained itself must read as running: %+v", latest)
	}
	if err := os.Remove(filepath.Join(dir, "bbbb2222.json")); err != nil {
		t.Fatal(err)
	}
	latest, ok = latestDesktopExitNote(desktopRunID)
	if !ok || latest.RunID != "aaaa1111" {
		t.Fatalf("latest = %+v (%v), want the remaining predecessor", latest, ok)
	}
}

func TestPruneDesktopExitNotesBoundsTheStore(t *testing.T) {
	dir := exitNoteDirForTest(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	desktopExitNoteNow = func() time.Time { return now }

	const extra = 5
	for i := 0; i < maxDesktopExitNotes+extra; i++ {
		path := filepath.Join(dir, strings.Repeat("a", 3)+string(rune('a'+i%26))+string(rune('0'+i/26))+"x.json")
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := now.Add(-time.Duration(maxDesktopExitNotes+extra-i) * time.Hour)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	stale := filepath.Join(dir, "stale.json")
	if err := os.WriteFile(stale, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-desktopExitNoteRetention - time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	pruneDesktopExitNotes()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("a note past the retention window survived the prune")
	}
	if len(entries) != maxDesktopExitNotes {
		t.Fatalf("kept %d notes, want %d", len(entries), maxDesktopExitNotes)
	}
}
