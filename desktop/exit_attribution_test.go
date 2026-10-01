package main

import (
	"os"
	"testing"
	"time"
)

// theRoundBefore is one predecessor run recorded the way a real one would be: a
// note the process wrote and an observation the watcher wrote. What the launch
// does with those two is the whole feature.
func theRoundBefore(t *testing.T, note desktopExitNote, observation desktopExitObservation, writeObservation bool) {
	t.Helper()
	exitNoteDirForTest(t)
	exitObservationDirForTest(t)
	if !writeDesktopExitNote(note) {
		t.Fatalf("could not write the predecessor note %s", note.RunID)
	}
	if writeObservation && !recordDesktopExitObservation(observation) {
		t.Fatal("could not write the predecessor observation")
	}
}

func TestRecordPreviousRunExitNamesACleanQuit(t *testing.T) {
	theRoundBefore(t,
		desktopExitNote{RunID: "prev", PID: 4242, Kind: exitKindClean, Reason: "host shut down",
			StartedAt: time.Now().Add(-time.Hour).Format(time.RFC3339Nano),
			ExitedAt:  time.Now().Add(-time.Minute).Format(time.RFC3339Nano)},
		desktopExitObservation{RunID: "prev", PID: 4242, ExitCode: 0, CodeKnown: true}, true)

	verdict := recordPreviousRunExit()
	if verdict.Kind != exitKindClean {
		t.Fatalf("verdict = %+v, want a clean quit", verdict)
	}
	if verdict.RunID != "prev" || verdict.PID != 4242 {
		t.Fatalf("the verdict does not name the run it explains: %+v", verdict)
	}
}

// The case this whole feature exists for: the process was killed and could not say
// so, and the launch still has to be able to name what happened.
func TestRecordPreviousRunExitNamesAnExternalKill(t *testing.T) {
	theRoundBefore(t,
		desktopExitNote{RunID: "prev", PID: 5151, Kind: exitKindRunning,
			StartedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)},
		desktopExitObservation{RunID: "prev", PID: 5151, ExitCode: 1, CodeKnown: true}, true)

	verdict := recordPreviousRunExit()
	if verdict.Kind != exitKindKilled {
		t.Fatalf("verdict = %+v, want a kill", verdict)
	}
	if verdict.ExitCode != "0x1" {
		t.Fatalf("the verdict lost the exit code: %+v", verdict)
	}
}

func TestRecordPreviousRunExitNamesAMachineStopping(t *testing.T) {
	theRoundBefore(t,
		desktopExitNote{RunID: "prev", PID: 6262, Kind: exitKindRunning,
			StartedAt: time.Now().Add(-3 * time.Hour).Format(time.RFC3339Nano)},
		desktopExitObservation{}, false)

	verdict := recordPreviousRunExit()
	if verdict.Kind != exitKindGone {
		t.Fatalf("verdict = %+v, want the unexplained kind", verdict)
	}
	if verdict.Reason == "" {
		t.Fatal("an unexplained run still needs a reason a person can read")
	}
}

func TestRecordPreviousRunExitFindsNothingOnAFirstRun(t *testing.T) {
	exitNoteDirForTest(t)
	exitObservationDirForTest(t)

	if verdict := recordPreviousRunExit(); verdict.Kind != exitKindGone {
		t.Fatalf("verdict = %+v, want the unexplained kind when nothing was recorded", verdict)
	}
}

// The verdict has to survive where a person can read it, with no telemetry switch
// in the way: the marker the watchdog already writes is that place.
func TestHostStateMarkerCarriesTheLastExitVerdict(t *testing.T) {
	exitNoteDirForTest(t)
	exitObservationDirForTest(t)
	hostStatePathForTest(t)

	verdict := recordPreviousRunExit()
	noteHostLaunch(false)

	record, ok := readHostState()
	if !ok {
		t.Fatal("the launch wrote no host-state marker")
	}
	if record.LastExit == nil {
		t.Fatal("the marker carries no verdict about the previous run")
	}
	if record.LastExit.Kind != verdict.Kind || record.LastExit.Reason == "" {
		t.Fatalf("marker verdict = %+v, want the classification that was just made", record.LastExit)
	}
	if record.LastExit.UncleanStreak != record.UncleanStreak {
		t.Fatalf("the verdict disagrees with the streak it belongs to: %+v", record.LastExit)
	}
}

func TestLastDesktopExitObservationPicksTheNewestLine(t *testing.T) {
	exitObservationDirForTest(t)
	restoreNow := desktopExitObserverNow
	t.Cleanup(func() { desktopExitObserverNow = restoreNow })
	now := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	for i, runID := range []string{"old", "new"} {
		desktopExitObserverNow = func() time.Time { return now.Add(time.Duration(i) * time.Minute) }
		if !recordDesktopExitObservation(desktopExitObservation{RunID: runID, PID: 100 + i, CodeKnown: true}) {
			t.Fatalf("could not record %s", runID)
		}
	}
	observation, ok := lastDesktopExitObservation()
	if !ok || observation.RunID != "new" {
		t.Fatalf("observation = %+v (%v), want the newest line", observation, ok)
	}
	if path := desktopExitObservationPath(); path == "" {
		t.Fatal("the store has no path")
	}
	if _, err := os.Stat(desktopExitObservationPath()); err != nil {
		t.Fatalf("the store file is missing: %v", err)
	}
}
