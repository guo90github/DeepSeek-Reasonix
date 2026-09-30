package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// hostStateTestPath makes the marker hermetic: a temp file plus a dead-PID
// answer, so no test ever touches the real user state directory.
func hostStateTestPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), hostStateFileName)
	previousPath, previousAlive := hostStatePathFunc, hostStateProcessAlive
	hostStatePathFunc = func() string { return path }
	hostStateProcessAlive = func(int) bool { return false }
	t.Cleanup(func() {
		hostStatePathFunc, hostStateProcessAlive = previousPath, previousAlive
	})
	return path
}

func TestHostStateMarkerSilentBeforeTheFirstRun(t *testing.T) {
	hostStateTestPath(t)
	if hostStateBeforeLaunch().Seen {
		t.Fatal("no marker means no previous run to judge")
	}
	if hostCrashedLastRun() || hostCrashLoopDegraded() {
		t.Fatal("a machine that never ran must not look like a crash loop")
	}
}

func TestHostStateMarkerCleanExitLeavesNothingToResume(t *testing.T) {
	path := hostStateTestPath(t)
	seen := newHostStateSeen("run-1", "v0", true, 0)
	if err := seen.note("running", true); err != nil {
		t.Fatalf("note running: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a running host must leave a marker: %v", err)
	}
	if err := clearHostState(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if hostStateBeforeLaunch().Seen {
		t.Fatal("a clean exit must leave nothing behind, or the watchdog restarts a host the user closed")
	}
}

func TestHostStateMarkerDeadPIDIsAnUncleanExit(t *testing.T) {
	hostStateTestPath(t)
	seen := newHostStateSeen("run-1", "v0", true, 0)
	if err := seen.note("running", true); err != nil {
		t.Fatalf("note running: %v", err)
	}
	out := hostStateBeforeLaunch()
	if !out.Seen || !out.Dead {
		t.Fatalf("outcome = %+v, want a seen dead record", out)
	}
	if !out.Unattended {
		t.Fatal("the desired unattended state must survive the crash")
	}
	if out.UncleanStreak != 1 {
		t.Fatalf("streak = %d, want 1", out.UncleanStreak)
	}
	if hostCrashLoopDegraded() {
		t.Fatal("one crash is not a crash loop")
	}
}

func TestHostStateMarkerLivePIDIsNotACrash(t *testing.T) {
	hostStateTestPath(t)
	seen := newHostStateSeen("run-1", "v0", true, 0)
	if err := seen.note("running", true); err != nil {
		t.Fatalf("note running: %v", err)
	}
	hostStateProcessAlive = func(int) bool { return true }
	if hostStateBeforeLaunch().Dead {
		t.Fatal("a marker whose host is alive is not a crash")
	}
}

func TestHostStateMarkerCrashLoopGrowsUntilDegraded(t *testing.T) {
	hostStateTestPath(t)
	rec := hostStateRecord{
		SchemaVersion: hostStateSchemaVersion,
		PID:           424242,
		Phase:         "running",
		StartedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		UncleanStreak: hostCrashStreakLimit - 1,
	}
	if err := writeHostState(rec); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := hostStateBeforeLaunch()
	if out.UncleanStreak != hostCrashStreakLimit {
		t.Fatalf("streak = %d, want %d", out.UncleanStreak, hostCrashStreakLimit)
	}
	if !hostCrashLoopDegraded() {
		t.Fatal("a repeated crash inside the window must degrade this launch")
	}
}

func TestHostStateMarkerStaleRecordDoesNotAccumulate(t *testing.T) {
	hostStateTestPath(t)
	rec := hostStateRecord{
		SchemaVersion: hostStateSchemaVersion,
		PID:           424242,
		Phase:         "running",
		StartedAt:     time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano),
		UncleanStreak: 9,
	}
	if err := writeHostState(rec); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := hostStateBeforeLaunch().UncleanStreak; got != 1 {
		t.Fatalf("streak = %d, want 1: a stale record is not a crash loop", got)
	}
	if hostCrashLoopDegraded() {
		t.Fatal("an old record must not degrade a later launch")
	}
}

func TestHostStateMarkerIgnoresAFutureSchema(t *testing.T) {
	path := hostStateTestPath(t)
	future := `{"schemaVersion":99,"pid":1,"phase":"running"}`
	if err := os.WriteFile(path, []byte(future), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if hostStateBeforeLaunch().Seen {
		t.Fatal("a marker this binary cannot understand must read as no record")
	}
}
