package main

import (
	"fmt"
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

// A crash-degraded launch drives nothing, and recording that as "unattended: off" is what used
// to stop the OS watchdog from pulling this host back up — the very run that would reset the
// streak. So the marker has to ask for the switch a person set, not for this launch's decision.
func TestTheLaunchMarkerAsksForUnattendedFromTheSwitch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", home)
	hostStateTestPath(t)
	switchPath := filepath.Join(home, "heartbeat-tasks.json")
	writeSwitch := func(on bool) {
		t.Helper()
		if err := os.WriteFile(switchPath, []byte(fmt.Sprintf(`{"unattended":%t}`, on)), 0o600); err != nil {
			t.Fatalf("write switch: %v", err)
		}
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	// A predecessor that kept dying inside the window: this launch is degraded.
	if err := writeHostState(hostStateRecord{PID: 4242, Phase: "exited", StartedAt: startedAt, UncleanStreak: hostCrashStreakLimit}); err != nil {
		t.Fatalf("seed the crash loop: %v", err)
	}
	if !hostCrashLoopDegraded() {
		t.Fatal("premise: a dead predecessor inside the window must keep driving off")
	}

	writeSwitch(true)
	noteHostLaunch()
	if rec, ok := readHostState(); !ok || !rec.Unattended {
		t.Fatalf("marker = %+v (%v), want it to ask for unattended care while the switch is on", rec, ok)
	}
	writeSwitch(false)
	noteHostLaunch()
	if rec, _ := readHostState(); rec.Unattended {
		t.Fatal("marker asks for unattended care while the switch is off")
	}
	// An unreadable switch leaves the previous answer standing.
	if err := os.Remove(switchPath); err != nil {
		t.Fatalf("remove the switch: %v", err)
	}
	if err := writeHostState(hostStateRecord{PID: 4243, Phase: "exited", Unattended: true, StartedAt: startedAt}); err != nil {
		t.Fatalf("seed the previous answer: %v", err)
	}
	noteHostLaunch()
	if rec, _ := readHostState(); !rec.Unattended {
		t.Fatal("an unreadable switch dropped the previous answer instead of carrying it forward")
	}
}
