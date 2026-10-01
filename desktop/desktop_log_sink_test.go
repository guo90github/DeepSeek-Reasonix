package main

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installLogSinkForTest(t *testing.T, dir string, now time.Time) {
	t.Helper()
	restoreDir, restoreNow := desktopLogDirFunc, desktopLogNow
	restoreWriter, restoreDefault := log.Writer(), slog.Default()
	t.Cleanup(func() {
		desktopLogDirFunc, desktopLogNow = restoreDir, restoreNow
		log.SetOutput(restoreWriter)
		slog.SetDefault(restoreDefault)
		closeDesktopLogSink()
	})
	desktopLogDirFunc = func() string { return dir }
	desktopLogNow = func() time.Time { return now }
	installDesktopLogSink()
}

func desktopLogFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "desktop-") {
			names = append(names, entry.Name())
		}
	}
	return names
}

// A host log nobody can read after the fact is the whole problem: both the app
// logger and the watchdog logger have to land in the day file.
func TestDesktopLogSinkCapturesBothLoggers(t *testing.T) {
	dir := t.TempDir()
	installLogSinkForTest(t, dir, time.Date(2026, 10, 2, 3, 4, 5, 0, time.UTC))

	log.Printf("desktop-log-sentinel")
	slog.Info("desktop-slog-sentinel")

	body, err := os.ReadFile(filepath.Join(dir, "desktop-20261002.log"))
	if err != nil {
		t.Fatalf("day file: %v", err)
	}
	for _, want := range []string{"desktop-log-sentinel", "desktop-slog-sentinel"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("the day file is missing %q:\n%s", want, body)
		}
	}
}

func TestDesktopLogSinkPrunesOldDaysAndKeepsRecentOnes(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 2, 3, 4, 5, 0, time.UTC)
	stale := filepath.Join(dir, "desktop-20260901.log")
	recent := filepath.Join(dir, "desktop-20261001.log")
	for _, path := range []string{stale, recent} {
		if err := os.WriteFile(path, []byte("line\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := now.Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	installLogSinkForTest(t, dir, now)

	files := desktopLogFiles(t, dir)
	for _, name := range files {
		if name == "desktop-20260901.log" {
			t.Fatalf("a 40-day-old log survived the prune: %v", files)
		}
	}
	if len(files) != 2 {
		t.Fatalf("log files = %v, want the day file and the recent one", files)
	}
}
