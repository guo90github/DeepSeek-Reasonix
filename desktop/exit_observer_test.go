package main

import (
	"fmt"
	"os"
	"testing"
)

func exitObservationDirForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restore := desktopExitObservationDirFunc
	t.Cleanup(func() { desktopExitObservationDirFunc = restore })
	desktopExitObservationDirFunc = func() string { return dir }
	return dir
}

func TestExitObservationStoreKeepsOneReadableLinePerRun(t *testing.T) {
	exitObservationDirForTest(t)

	if !recordDesktopExitObservation(desktopExitObservation{RunID: "run-a", PID: 11, ExitCode: 0xC000013A, CodeKnown: true}) {
		t.Fatal("the observation was not recorded")
	}
	if !recordDesktopExitObservation(desktopExitObservation{RunID: "run-b", PID: 22, ExitCode: 1, CodeKnown: true}) {
		t.Fatal("the second observation was not recorded")
	}

	found, ok := latestDesktopExitObservationFor("run-a", 0)
	if !ok || found.ExitCode != 0xC000013A || found.PID != 11 {
		t.Fatalf("run-a observation = %+v (%v)", found, ok)
	}
	if _, ok := latestDesktopExitObservationFor("missing", 0); ok {
		t.Fatal("an unobserved run must not read as observed")
	}
	if observations := readDesktopExitObservations(); len(observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(observations))
	}
}

// A damaged line must not hide the ones around it: this store is the only place
// an externally killed run is written down at all.
func TestExitObservationStoreSkipsDamagedLines(t *testing.T) {
	exitObservationDirForTest(t)
	if !recordDesktopExitObservation(desktopExitObservation{RunID: "run-ok", PID: 7, ExitCode: 3, CodeKnown: true}) {
		t.Fatal("the observation was not recorded")
	}
	file, err := os.OpenFile(desktopExitObservationPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{not json\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, ok := latestDesktopExitObservationFor("run-ok", 0); !ok {
		t.Fatal("a damaged line hid the readable observation")
	}
}

func TestPruneDesktopExitObservationsKeepsTheNewest(t *testing.T) {
	exitObservationDirForTest(t)
	for i := 0; i < maxDesktopExitObservations+7; i++ {
		if !recordDesktopExitObservation(desktopExitObservation{RunID: fmt.Sprintf("run-%03d", i), PID: i + 1}) {
			t.Fatalf("could not record run-%03d", i)
		}
	}
	observations := readDesktopExitObservations()
	if len(observations) != maxDesktopExitObservations {
		t.Fatalf("observations = %d, want %d", len(observations), maxDesktopExitObservations)
	}
	if last := observations[len(observations)-1].RunID; last != fmt.Sprintf("run-%03d", maxDesktopExitObservations+6) {
		t.Fatalf("the newest observation was pruned instead of the oldest: %s", last)
	}
}

// The note is what the process knew about itself and outranks any code; the
// observation is what is left when it knew nothing, and it has to name the
// ambiguity instead of guessing at it.
func TestClassifyDesktopRunPrefersTheNoteAndNamesWhatItCannotKnow(t *testing.T) {
	running := desktopExitNote{RunID: "r", Kind: exitKindRunning}
	cases := []struct {
		name      string
		note      desktopExitNote
		noteOK    bool
		observed  bool
		code      uint32
		codeKnown bool
		wantKind  string
	}{
		{name: "clean quit", note: desktopExitNote{Kind: exitKindClean, Reason: "host shut down"}, noteOK: true, wantKind: exitKindClean},
		{name: "recovered panic", note: desktopExitNote{Kind: exitKindPanic, Reason: "recovered panic at x"}, noteOK: true, wantKind: exitKindPanic},
		{name: "relaunch", note: desktopExitNote{Kind: exitKindSelf, Reason: "relaunch"}, noteOK: true, wantKind: exitKindSelf},
		{name: "nothing at all", wantKind: exitKindGone},
		{name: "unexplained and watched", note: running, noteOK: true, observed: true, code: 1, codeKnown: true, wantKind: exitKindKilled},
		{name: "console stop", note: running, noteOK: true, observed: true, code: 0xC000013A, codeKnown: true, wantKind: exitKindKilled},
		{name: "access violation", note: running, noteOK: true, observed: true, code: 0xC0000005, codeKnown: true, wantKind: exitKindPanic},
		{name: "no code on this platform", note: running, noteOK: true, observed: true, wantKind: exitKindKilled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observation := desktopExitObservation{ExitCode: tc.code, CodeKnown: tc.codeKnown}
			kind, reason := classifyDesktopRun(tc.note, tc.noteOK, observation, tc.observed)
			if kind != tc.wantKind {
				t.Fatalf("kind = %q (%s), want %q", kind, reason, tc.wantKind)
			}
			if reason == "" {
				t.Fatal("every classification needs a reason a person can read")
			}
		})
	}
}
