package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/proc"
)

// Exit observer — the detached watcher that can report an exit the process itself
// never got to write. A process cannot record its own termination, so "the window
// vanished and nothing says why" is only answerable from outside: the observer
// holds the host's pid, waits for it, and writes the code the OS reports.
//
// It runs detached on purpose. A watcher that dies with the process it watches
// would observe exactly the cases the note already covers.
const (
	exitObserverFlag               = "--exit-observer"
	exitObserverPIDFlag            = "--exit-observer-pid="
	exitObserverRunIDFlag          = "--exit-observer-run-id="
	desktopExitObservationFileName = "desktop-exit-observations.jsonl"
	// desktopExitObserverOffEnv disables the watcher for callers that launch the
	// host in a way they already observe themselves.
	desktopExitObserverOffEnv = "REASONIX_NO_EXIT_OBSERVER"
	// maxDesktopExitObservations bounds the file: one line per run, and the store
	// is a diagnostic, not a history.
	maxDesktopExitObservations = 50
	exitObservationSchema      = 1
)

// desktopExitObservation is what only an outsider can know about one run's end.
type desktopExitObservation struct {
	SchemaVersion int    `json:"schemaVersion"`
	RunID         string `json:"runId,omitempty"`
	PID           int    `json:"pid"`
	ObservedAt    string `json:"observedAt"`
	UptimeMS      int64  `json:"uptimeMs,omitempty"`
	ExitCode      uint32 `json:"exitCode"`
	// CodeKnown is false when the platform cannot report a code for a process it
	// does not own; the exit is still observed, its code just is not.
	CodeKnown bool `json:"codeKnown"`
}

var desktopExitObservationDirFunc = func() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return root
}

var desktopExitObserverNow = func() time.Time { return time.Now().UTC() }

func desktopExitObservationPath() string {
	dir := desktopExitObservationDirFunc()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, desktopExitObservationFileName)
}

// recordDesktopExitObservation appends one run's observed end and keeps the file
// bounded. The caller is the observer process, on its way out.
func recordDesktopExitObservation(observation desktopExitObservation) bool {
	path := desktopExitObservationPath()
	if path == "" {
		return false
	}
	observation.SchemaVersion = exitObservationSchema
	observation.ObservedAt = desktopExitObserverNow().Format(time.RFC3339Nano)
	body, err := json.Marshal(observation)
	if err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return false
	}
	_, writeErr := file.Write(append(body, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return false
	}
	pruneDesktopExitObservations(path)
	return true
}

// readDesktopExitObservations returns every line the file still holds. A damaged
// line is skipped: the store is a diagnostic, and one bad byte must not hide the
// rest of an incident.
func readDesktopExitObservations() []desktopExitObservation {
	path := desktopExitObservationPath()
	if path == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []desktopExitObservation
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var observation desktopExitObservation
		if err := json.Unmarshal([]byte(line), &observation); err != nil {
			continue
		}
		if observation.SchemaVersion > exitObservationSchema {
			continue
		}
		out = append(out, observation)
	}
	return out
}

// latestDesktopExitObservationFor finds the observation of one run, newest last
// in the file.
func latestDesktopExitObservationFor(runID string, pid int) (desktopExitObservation, bool) {
	var found desktopExitObservation
	ok := false
	for _, observation := range readDesktopExitObservations() {
		if runID != "" && observation.RunID == runID {
			found, ok = observation, true
			continue
		}
		if runID == "" && pid > 0 && observation.PID == pid {
			found, ok = observation, true
		}
	}
	return found, ok
}

func pruneDesktopExitObservations(path string) {
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) <= maxDesktopExitObservations {
		return
	}
	kept := lines[len(lines)-maxDesktopExitObservations:]
	_ = os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600)
}

// observeDesktopRunEnd waits for one process and records what the OS reports.
func observeDesktopRunEnd(pid int, runID string, startedAt time.Time) bool {
	if pid <= 0 {
		return false
	}
	code, known := waitDesktopExitCode(pid)
	observation := desktopExitObservation{RunID: runID, PID: pid, ExitCode: code, CodeKnown: known}
	if !startedAt.IsZero() {
		observation.UptimeMS = desktopExitObserverNow().Sub(startedAt).Milliseconds()
	}
	return recordDesktopExitObservation(observation)
}

// spawnDesktopExitObserver starts the detached watcher for this run, unless the
// platform has no way to report a code or a caller asked for none.
func spawnDesktopExitObserver() {
	if !desktopExitObserverAvailable() || strings.TrimSpace(os.Getenv(desktopExitObserverOffEnv)) != "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := proc.Command(exe,
		exitObserverFlag,
		exitObserverPIDFlag+strconv.Itoa(os.Getpid()),
		exitObserverRunIDFlag+desktopRunID)
	if err := cmd.Start(); err != nil {
		log.Printf("[desktop] exit observer could not start: %v", err)
		return
	}
	// The watcher outlives this process; nothing here waits for it.
	_ = cmd.Process.Release()
}

// maybeRunExitObserver owns the invocation when this process is the watcher, so
// the host's own startup never runs twice in it.
func maybeRunExitObserver(args []string) (bool, int) {
	mode := false
	pid := 0
	runID := ""
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		switch {
		case trimmed == exitObserverFlag:
			mode = true
		case strings.HasPrefix(trimmed, exitObserverPIDFlag):
			pid, _ = strconv.Atoi(strings.TrimPrefix(trimmed, exitObserverPIDFlag))
		case strings.HasPrefix(trimmed, exitObserverRunIDFlag):
			runID = strings.TrimPrefix(trimmed, exitObserverRunIDFlag)
		}
	}
	if !mode {
		return false, 0
	}
	observeDesktopRunEnd(pid, runID, time.Time{})
	return true, 0
}

// classifyDesktopRun turns one run's own note plus the outsider's observation into
// the single answer to "how did that run end". Ambiguity is named, never guessed:
// a code with no note is a termination, but which code means what depends on who
// did the terminating.
func classifyDesktopRun(note desktopExitNote, noteOK bool, observation desktopExitObservation, observationOK bool) (string, string) {
	if noteOK {
		switch note.Kind {
		case exitKindClean, exitKindPanic, exitKindSelf:
			return note.Kind, note.Reason
		}
	}
	if !observationOK {
		// Nobody watched this run end: the machine or the watcher went down with it.
		return exitKindGone, "no exit was observed: the machine or the watcher went down with the run"
	}
	if kind, reason, ok := desktopHardExitKind(observation); ok {
		return kind, reason
	}
	if !observation.CodeKnown {
		return exitKindKilled, "the run ended without a code the platform could report"
	}
	return exitKindKilled, fmt.Sprintf("the process ended with code %s without recording why", desktopExitCodeText(observation.ExitCode))
}

// desktopHardExitKind names the codes that identify their own cause. Everything
// else is a termination whose cause only the code hints at.
func desktopHardExitKind(observation desktopExitObservation) (string, string, bool) {
	if !observation.CodeKnown {
		return "", "", false
	}
	switch observation.ExitCode {
	case 0xC0000005:
		return exitKindPanic, "native fault: 0xC0000005 access violation", true
	case 0xC000001D:
		return exitKindPanic, "native fault: 0xC000001D illegal instruction", true
	case 0xC0000409:
		return exitKindPanic, "native fault: 0xC0000409 stack buffer overrun", true
	case 0xC000013A:
		return exitKindKilled, "0xC000013A: the console or the parent asked it to stop", true
	case 0xC0000017:
		return exitKindKilled, "0xC0000017: the machine was out of memory", true
	}
	return "", "", false
}

func desktopExitCodeText(code uint32) string {
	return "0x" + strconv.FormatUint(uint64(code), 16)
}
