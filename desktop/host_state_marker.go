// Host state marker — the durable "this host was running" record behind
// unattended keep-alive: written on every launch, removed only by a clean exit,
// and read by the restart policy and the OS watchdog.

package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
)

const (
	hostStateSchemaVersion = 1
	hostStateFileName      = "desktop-host-state.json"
	// hostCrashStreakWindow bounds how close two unclean exits must be to count
	// as one crash loop instead of two unrelated incidents.
	hostCrashStreakWindow = 10 * time.Minute
	// hostCrashStreakLimit is the point where the host stops trusting this
	// launch to survive one: unattended driving stays off for that run so a
	// crashing task cannot keep killing the app.
	hostCrashStreakLimit = 3
)

type hostStateRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	PID           int    `json:"pid"`
	RunID         string `json:"runId,omitempty"`
	Version       string `json:"version,omitempty"`
	Phase         string `json:"phase"`
	Unattended    bool   `json:"unattended"`
	StartedAt     string `json:"startedAt"`
	UpdatedAt     string `json:"updatedAt"`
	// UncleanStreak counts consecutive launches that found a dead predecessor
	// inside hostCrashStreakWindow.
	UncleanStreak int `json:"uncleanStreak,omitempty"`
	// LastExit says how the run before this one ended, decided by the attribution
	// and readable without telemetry: it is what a person asking "why did it
	// vanish again" has to be able to look up.
	LastExit *desktopExitVerdict `json:"lastExit,omitempty"`
}

// hostStateProcessAlive is a seam for tests: a marker whose PID is alive belongs
// to a running host, not to a crash.
var hostStateProcessAlive = desktopProcessAlive

// hostStatePathFunc is a seam for tests; production always resolves the user
// state directory.
var hostStatePathFunc = defaultHostStatePath

func hostStatePath() string { return hostStatePathFunc() }

func defaultHostStatePath() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, hostStateFileName)
}

// readHostState returns the record left by a previous launch.
func readHostState() (hostStateRecord, bool) {
	path := hostStatePath()
	if path == "" {
		return hostStateRecord{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return hostStateRecord{}, false
	}
	var rec hostStateRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return hostStateRecord{}, false
	}
	if rec.SchemaVersion > hostStateSchemaVersion {
		// A newer host wrote this; guessing its semantics is worse than
		// treating the launch as attended.
		return hostStateRecord{}, false
	}
	return rec, true
}

func writeHostState(rec hostStateRecord) error {
	path := hostStatePath()
	if path == "" {
		return errors.New("no user state directory")
	}
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = hostStateSchemaVersion
	}
	rec.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, body, 0o600)
}

func clearHostState() error {
	path := hostStatePath()
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// hostStateSeen is the marker the running host owns; every later rewrite (a
// phase change) reuses it instead of re-deriving pid and start time.
type hostStateSeen struct {
	mu  sync.Mutex
	rec hostStateRecord
}

func newHostStateSeen(runID, appVersion string, unattended bool, streak int) *hostStateSeen {
	record := hostStateRecord{
		SchemaVersion: hostStateSchemaVersion,
		PID:           os.Getpid(),
		RunID:         runID,
		Version:       appVersion,
		Phase:         "starting",
		Unattended:    unattended,
		StartedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		UncleanStreak: streak,
	}
	if previousRunExit != nil {
		verdict := *previousRunExit
		verdict.UncleanStreak = streak
		record.LastExit = &verdict
	}
	return &hostStateSeen{rec: record}
}

func (s *hostStateSeen) note(phase string, unattended bool) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Phase = phase
	s.rec.Unattended = unattended
	return writeHostState(s.rec)
}

// hostStateOutcome is what the previous launch left behind.
type hostStateOutcome struct {
	Seen          bool
	PID           int
	Unattended    bool
	Dead          bool // the record exists and its process is gone
	UncleanStreak int
}

// hostStateBeforeLaunch reads the previous record and decides what it means.
// It never mutates the file: the caller owns the write that marks this launch.
func hostStateBeforeLaunch() hostStateOutcome {
	rec, ok := readHostState()
	if !ok {
		return hostStateOutcome{}
	}
	dead := rec.PID <= 0 || !hostStateProcessAlive(rec.PID)
	out := hostStateOutcome{Seen: true, PID: rec.PID, Unattended: rec.Unattended, Dead: dead}
	if !dead {
		return out
	}
	// A dead predecessor inside the window extends the streak; a stale one
	// starts over, because days-old records are not a crash loop.
	out.UncleanStreak = 1
	if started, err := time.Parse(time.RFC3339Nano, rec.StartedAt); err == nil {
		if time.Since(started) <= hostCrashStreakWindow {
			if rec.UncleanStreak > 0 {
				out.UncleanStreak = rec.UncleanStreak + 1
			}
		}
	}
	return out
}

// hostCrashedLastRun reports whether the previous launch died without clearing
// its marker.
func hostCrashedLastRun() bool {
	return hostStateBeforeLaunch().Dead
}

// hostCrashLoopDegraded reports whether this launch must keep unattended
// driving off, because the previous attempts to run it kept dying.
func hostCrashLoopDegraded() bool {
	out := hostStateBeforeLaunch()
	return out.Dead && out.UncleanStreak >= hostCrashStreakLimit
}

// noteHostLaunch records this launch: the marker the OS watchdog and the
// restart policy both read, carrying the desired unattended state forward.
func noteHostLaunch(unattended bool) {
	before := hostStateBeforeLaunch()
	if before.Seen && before.Dead {
		slog.Warn("desktop: previous host run did not shut down cleanly",
			"pid", before.PID, "uncleanStreak", before.UncleanStreak)
	}
	seen := newHostStateSeen(desktopRunID, version, unattended, before.UncleanStreak)
	if err := seen.note("running", unattended); err != nil {
		slog.Warn("desktop: write host-state marker", "err", err)
	}
}
