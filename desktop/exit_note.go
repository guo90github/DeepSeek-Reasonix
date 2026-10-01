// Exit notes — one durable record per host run saying how that run ended.
//
// Written unconditionally, never behind the telemetry switch that gates crash
// reports: "why did the window vanish" is asked by the person who was using it.
// A note holds what only the process itself knows (it quit on purpose, it
// panicked); what only an outsider knows (it was terminated, and with which
// exit code) arrives separately, from the exit observer.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
)

const (
	desktopExitNoteSchemaVersion = 1
	desktopExitNoteDirName       = "exit-notes"
	// The store is bounded by age and by count: a crash loop must not grow it.
	desktopExitNoteRetention = 30 * 24 * time.Hour
	maxDesktopExitNotes      = 20
)

// How a run ended when the process recorded it itself. A run nobody explained
// keeps exitKindRunning, which is what makes an unexplained disappearance
// distinguishable from a documented quit.
const (
	exitKindRunning = "running"
	exitKindClean   = "clean"
	exitKindPanic   = "panic"
	exitKindSelf    = "self"
	// The last two are never written by the process itself: only an outsider can
	// observe a termination, so the attribution decides between them.
	exitKindKilled = "killed"
	exitKindGone   = "gone"
)

type desktopExitNote struct {
	SchemaVersion int    `json:"schemaVersion"`
	RunID         string `json:"runId"`
	PID           int    `json:"pid"`
	Version       string `json:"version,omitempty"`
	Phase         string `json:"phase,omitempty"`
	Kind          string `json:"kind"`
	Reason        string `json:"reason,omitempty"`
	StartedAt     string `json:"startedAt,omitempty"`
	ExitedAt      string `json:"exitedAt,omitempty"`
}

// desktopRunID names this process everywhere an incident is recorded — the exit
// note, the exit observer and the host-state marker — so one incident reads as
// one run everywhere instead of as three unrelated ids.
var desktopRunID = newDesktopLifecycleRunID()

// desktopRunStartedAt is when this process began; the note's uptime derives from it.
var desktopRunStartedAt = time.Now().UTC()

// desktopExitNoteDirFunc is a test seam; production keeps notes in the user state
// directory beside the host-state marker they explain.
var desktopExitNoteDirFunc = func() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, desktopExitNoteDirName)
}

var desktopExitNoteNow = func() time.Time { return time.Now().UTC() }

func desktopExitNotePath(runID string) string {
	dir := desktopExitNoteDirFunc()
	if dir == "" || runID == "" {
		return ""
	}
	return filepath.Join(dir, runID+".json")
}

// noteDesktopRunStarted records this run before anything can kill it. A run whose
// note still says running ended without the process explaining itself.
func noteDesktopRunStarted() {
	pruneDesktopExitNotes()
	writeDesktopExitNote(desktopExitNote{
		RunID:     desktopRunID,
		PID:       os.Getpid(),
		Version:   version,
		Phase:     "starting",
		Kind:      exitKindRunning,
		StartedAt: desktopRunStartedAt.Format(time.RFC3339Nano),
	})
}

// noteDesktopRunExited rewrites this run's note with how it is ending. Every
// caller is on its way out, so a failure costs the explanation and nothing else.
func noteDesktopRunExited(kind, reason, phase string) {
	note, ok := readDesktopExitNote(desktopRunID)
	if !ok {
		note = desktopExitNote{
			RunID:     desktopRunID,
			PID:       os.Getpid(),
			Version:   version,
			StartedAt: desktopRunStartedAt.Format(time.RFC3339Nano),
		}
	}
	note.Kind = kind
	note.Reason = reason
	if phase != "" {
		note.Phase = phase
	}
	note.ExitedAt = desktopExitNoteNow().Format(time.RFC3339Nano)
	writeDesktopExitNote(note)
}

func writeDesktopExitNote(note desktopExitNote) bool {
	path := desktopExitNotePath(note.RunID)
	if path == "" {
		return false
	}
	note.SchemaVersion = desktopExitNoteSchemaVersion
	body, err := json.Marshal(note)
	if err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false
	}
	return fileutil.AtomicWriteFile(path, body, 0o600) == nil
}

// readDesktopExitNote decodes one run's note. Damage is never fatal: a note that
// cannot be read is one explanation lost, not a reason to fail a launch.
func readDesktopExitNote(runID string) (desktopExitNote, bool) {
	path := desktopExitNotePath(runID)
	if path == "" {
		return desktopExitNote{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return desktopExitNote{}, false
	}
	var note desktopExitNote
	if err := json.Unmarshal(body, &note); err != nil {
		return desktopExitNote{}, false
	}
	if note.SchemaVersion > desktopExitNoteSchemaVersion {
		return desktopExitNote{}, false
	}
	return note, true
}

// latestDesktopExitNote returns the newest note that is not this run's, so a
// launch can explain its predecessor. Newest means by exit time, and by file time
// when a run never got to write one.
func latestDesktopExitNote(excludeRunID string) (desktopExitNote, bool) {
	dir := desktopExitNoteDirFunc()
	if dir == "" {
		return desktopExitNote{}, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return desktopExitNote{}, false
	}
	type candidate struct {
		note  desktopExitNote
		order time.Time
	}
	var found []candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		runID := entry.Name()
		if index := len(runID) - len(".json"); index > 0 && filepath.Ext(runID) == ".json" {
			runID = runID[:index]
		} else {
			continue
		}
		if runID == excludeRunID {
			continue
		}
		note, ok := readDesktopExitNote(runID)
		if !ok {
			continue
		}
		order, err := time.Parse(time.RFC3339Nano, note.ExitedAt)
		if err != nil {
			if info, statErr := entry.Info(); statErr == nil {
				order = info.ModTime()
			}
		}
		found = append(found, candidate{note: note, order: order})
	}
	if len(found) == 0 {
		return desktopExitNote{}, false
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].order.After(found[j].order) })
	return found[0].note, true
}

// pruneDesktopExitNotes keeps the store bounded: notes past the retention window
// go first, then the oldest beyond the count limit.
func pruneDesktopExitNotes() {
	dir := desktopExitNoteDirFunc()
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := desktopExitNoteNow().Add(-desktopExitNoteRetention)
	type aged struct {
		path     string
		modified time.Time
	}
	var kept []aged
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
			continue
		}
		kept = append(kept, aged{path: path, modified: info.ModTime()})
	}
	if len(kept) <= maxDesktopExitNotes {
		return
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].modified.After(kept[j].modified) })
	for _, entry := range kept[maxDesktopExitNotes:] {
		_ = os.Remove(entry.path)
	}
}
