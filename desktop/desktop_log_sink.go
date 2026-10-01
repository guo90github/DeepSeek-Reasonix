package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/config"
)

// desktopLogRetention keeps a week of host logs: enough to read back an incident
// that happened over a weekend, far from unbounded growth.
const desktopLogRetention = 7 * 24 * time.Hour

// desktopLogDirFunc is a test seam. Production writes under the cache root, so
// the host's own logs never share a directory with sessions or settings.
var desktopLogDirFunc = func() string {
	root := config.CacheDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "logs")
}

var desktopLogNow = func() time.Time { return time.Now().UTC() }

// desktopLogSinkFile is the handle the sink keeps open for the process lifetime.
// It is a package variable so a test can release it: on Windows an open handle
// blocks the temp directory's own cleanup.
var desktopLogSinkFile *os.File

// closeDesktopLogSink releases the handle. The production sink never calls it.
func closeDesktopLogSink() {
	if desktopLogSinkFile == nil {
		return
	}
	_ = desktopLogSinkFile.Close()
	desktopLogSinkFile = nil
}

// installDesktopLogSink gives the host's own logging a durable file. Without it
// every line the host writes — heartbeat decisions, upgrade relaunches, shutdown
// teardown — goes to a stderr nobody reads when the app is started from a
// launcher, which is why a vanished desktop left no explanation behind.
//
// Both loggers are redirected on purpose: the watchdog reports through slog and
// the app through log, and one incident spans both. The file is named for the
// launch day, so a host that runs for a week keeps appending to one readable file.
func installDesktopLogSink() {
	dir := desktopLogDirFunc()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(dir, "desktop-"+desktopLogNow().Format("20060102")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	closeDesktopLogSink()
	desktopLogSinkFile = file
	pruneDesktopLogs(dir, path)
	sink := io.MultiWriter(file, os.Stderr)
	log.SetOutput(sink)
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	log.Printf("desktop: logging to %s (pid %d, version %s, channel %s)", path, os.Getpid(), version, channel)
}

// pruneDesktopLogs drops day files past the retention window. The file this
// launch just opened is never a candidate, whatever the clock says about it.
func pruneDesktopLogs(dir, current string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := desktopLogNow().Add(-desktopLogRetention)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "desktop-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		path := filepath.Join(dir, name)
		if path == current {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(path)
	}
}
