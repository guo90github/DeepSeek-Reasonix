// Watchdog management — the OS entry that restores a host the machine lost is
// owned here, with one implementation behind three surfaces: applied on every
// start, driven by the CLI, or toggled from the UI.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
	"reasonix/internal/installlayout"
)

const watchdogTaskName = "ReasonixDesktopWatchdog"

// watchdogPlatformRunner runs one scheduler command; it is a seam for tests.
var watchdogPlatformRunner = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type watchdogPolicy struct {
	SchemaVersion int  `json:"schemaVersion,omitempty"`
	Enabled       bool `json:"enabled"`
	// OptOut records an explicit "leave this machine's scheduler alone" — written only by
	// `--watchdog-disable`. It is the one thing that keeps the OS entry off: the entry used
	// to follow the unattended switch, which unregistered it on every machine that did not
	// run unattended — exactly the machines whose dead host a phone now wants back
	// (2026-10-05). The old `watchdog` field is gone: a legacy `watchdog:false` came from
	// that switch, so reading it as an opt-out would silently disable the new behaviour.
	OptOut bool `json:"optOut,omitempty"`
}

func watchdogPolicyPath() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "desktop-autostart.json")
}

// watchdogEntryWanted is the **OS entry's** own policy: on unless someone explicitly
// opted out. It deliberately ignores `Enabled`, which is a different signal — that field
// is the login item's (the master switch's mirror, read by the Electron shell in
// `autostart.ts`), and the entry no longer follows it (2026-10-05). Keeping the two
// apart is what stops a phone-facing wake entry from also turning on login autostart for
// a machine that never asked.
func watchdogEntryWanted(policy watchdogPolicy) bool {
	return !policy.OptOut
}

func readWatchdogPolicy() watchdogPolicy {
	path := watchdogPolicyPath()
	if path == "" {
		return watchdogPolicy{}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return watchdogPolicy{}
	}
	var policy watchdogPolicy
	if err := json.Unmarshal(body, &policy); err != nil {
		return watchdogPolicy{}
	}
	return policy
}

func writeWatchdogPolicy(policy watchdogPolicy) error {
	path := watchdogPolicyPath()
	if path == "" {
		return errors.New("no user state directory")
	}
	policy.SchemaVersion = 1
	body, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(body, '\n'), 0o600)
}

// watchdogEntryPoint is the binary the scheduler actually runs: the install
// root's stable launcher, which resolves the active version on every run. The
// script writes this same path, so what a person reads is what runs.
func watchdogEntryPoint() string {
	root := portableInstallRoot()
	if root == "" {
		return ""
	}
	path, err := installlayout.StableRelaunchPath(root)
	if err != nil {
		return ""
	}
	return path
}

func watchdogLaunchAgentPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", "io.reasonix.desktop.watchdog.plist")
}

func watchdogLaunchAgentBody(script string) string {
	return strings.Join([]string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
		`<plist version="1.0">`,
		`<dict>`,
		`  <key>Label</key><string>io.reasonix.desktop.watchdog</string>`,
		`  <key>ProgramArguments</key>`,
		`  <array><string>/bin/sh</string><string>` + script + `</string></array>`,
		`  <key>RunAtLoad</key><true/>`,
		`  <key>StartInterval</key><integer>300</integer>`,
		`  <key>ProcessType</key><string>Background</string>`,
		`</dict>`,
		`</plist>`,
		``,
	}, "\n")
}

func watchdogSupportedPlatform() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// watchdogIntervalMinutes is how often the scheduler runs the watchdog, and therefore
// the upper bound on how long a host that died can stay down. It was 5 while only
// unattended runs were restored; a phone reaching for a dead desktop makes the wait the
// whole experience, so the tick is one minute (2026-10-05). Each tick is a process that
// reads one small JSON file and exits, so the tighter cadence costs nothing measurable.
const watchdogIntervalMinutes = 1

// watchdogRegisterCommand is the Windows registration as one PowerShell command. The
// settings are the point of it: with Windows' defaults the task inherits "do not start
// on battery power" and "do not catch up a missed run", so it is accepted, reported as
// registered, and never runs. The trigger repeats indefinitely, so the entry survives
// a missed tick.
func watchdogRegisterCommand(task, script string) string {
	return strings.Join([]string{
		"$a = New-ScheduledTaskAction -Execute '" + psQuote(script) + "'",
		"$t = New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes " +
			strconv.Itoa(watchdogIntervalMinutes) + ")",
		"$s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew",
		"Register-ScheduledTask -TaskName '" + psQuote(task) + "' -Action $a -Trigger $t -Settings $s -Force | Out-Null",
	}, "; ")
}

// psQuote escapes a value for a single-quoted PowerShell string.
func psQuote(value string) string { return strings.ReplaceAll(value, "'", "''") }

// applyWatchdogRegistration registers or removes the OS entry and reports what
// the machine now says. It is idempotent: every start calls it.
func applyWatchdogRegistration(enabled bool) (bool, error) {
	switch runtime.GOOS {
	case "windows":
		if enabled {
			script := watchdogScriptPath()
			if script == "" {
				return false, errors.New("no home directory for the watchdog script")
			}
			if err := writeWatchdogScript(); err != nil {
				return false, err
			}
			// The action is the script (never a version), and registration goes through
			// PowerShell: with `schtasks /Create` the task stayed registered and never ran
			// (2026-10-02), which the status view cannot tell from a working watchdog.
			out, err := watchdogPlatformRunner("powershell", "-NoProfile", "-NonInteractive",
				"-Command", watchdogRegisterCommand(watchdogTaskName, script))
			if err != nil {
				return false, fmt.Errorf("register the watchdog task: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return true, nil
		}
		out, err := watchdogPlatformRunner("schtasks", "/Delete", "/TN", watchdogTaskName, "/F")
		if err != nil && !strings.Contains(strings.ToLower(string(out)), "cannot find") {
			return false, fmt.Errorf("schtasks delete: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return false, nil
	case "darwin":
		path := watchdogLaunchAgentPath()
		if path == "" {
			return false, errors.New("no home directory for the LaunchAgent")
		}
		if enabled {
			script := watchdogScriptPath()
			if script == "" {
				return false, errors.New("no home directory for the watchdog script")
			}
			if err := writeWatchdogScript(); err != nil {
				return false, err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return false, err
			}
			if err := os.WriteFile(path, []byte(watchdogLaunchAgentBody(script)), 0o644); err != nil {
				return false, err
			}
			if out, err := watchdogPlatformRunner("launchctl", "load", "-w", path); err != nil {
				slog.Warn("desktop watchdog: launchctl load failed", "err", err, "output", strings.TrimSpace(string(out)))
			}
			return true, nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		_, _ = watchdogPlatformRunner("launchctl", "unload", "-w", path)
		return false, nil
	default:
		return false, nil
	}
}

// watchdogRegistrationPresent reports whether the OS entry exists right now,
// without changing anything.
func watchdogRegistrationPresent() bool {
	switch runtime.GOOS {
	case "windows":
		if _, err := watchdogPlatformRunner("schtasks", "/Query", "/TN", watchdogTaskName); err != nil {
			return false
		}
		return true
	case "darwin":
		path := watchdogLaunchAgentPath()
		if path == "" {
			return false
		}
		_, err := os.Stat(path)
		return err == nil
	default:
		return false
	}
}

// WatchdogStatusView is what the UI and the CLI both read.
type WatchdogStatusView struct {
	Supported  bool   `json:"supported"`
	Policy     bool   `json:"policy"`
	Registered bool   `json:"registered"`
	EntryPoint string `json:"entryPoint,omitempty"`
	Platform   string `json:"platform"`
	LastError  string `json:"lastError,omitempty"`
	Note       string `json:"note,omitempty"`
	// LastRunAt is when the machine last started the entry, RFC3339 UTC. Empty
	// while Registered: the entry exists and has never run — the difference
	// between a working watchdog and false comfort (2026-10-03).
	LastRunAt string `json:"lastRunAt,omitempty"`
}

// WatchdogStatus reports the policy, the machine, and the binary it would run.
func (a *App) WatchdogStatus() WatchdogStatusView {
	return watchdogStatusView()
}

func watchdogStatusView() WatchdogStatusView {
	policy := readWatchdogPolicy()
	view := WatchdogStatusView{
		Supported:  watchdogSupportedPlatform(),
		Policy:     watchdogEntryWanted(policy),
		Platform:   runtime.GOOS,
		EntryPoint: watchdogEntryPoint(),
	}
	if !view.Supported {
		view.Note = "this platform has no watchdog integration"
		return view
	}
	view.Registered = watchdogRegistrationPresent()
	if view.Registered {
		view.LastRunAt = watchdogLastRunAt()
	}
	return view
}

// watchdogLastRunAt is when the machine last started the entry. Windows answers
// from the scheduler, which is the authoritative "the trigger really fires" — the
// defect this view exists to expose (2026-10-02/03). Elsewhere the inspection log,
// which the watchdog appends to on every tick it runs, is the only record left.
func watchdogLastRunAt() string {
	if runtime.GOOS == "windows" {
		out, err := watchdogPlatformRunner("powershell", "-NoProfile", "-NonInteractive", "-Command",
			"(Get-ScheduledTaskInfo -TaskName '"+psQuote(watchdogTaskName)+"').LastRunTime.ToUniversalTime().ToString('o')")
		if err != nil {
			return ""
		}
		return watchdogRunTimeFromTaskInfo(string(out))
	}
	if at, ok := watchdogLastLogTime(); ok {
		return at.UTC().Format(time.RFC3339)
	}
	return ""
}

// watchdogRunTimeFromTaskInfo reads Get-ScheduledTaskInfo's LastRunTime. A task
// that never ran answers 1999-11-30, so anything before 2000 means "never".
func watchdogRunTimeFromTaskInfo(out string) string {
	body := strings.Trim(string(out), " \t\r\n\x00\ufeff")
	at, err := time.Parse(time.RFC3339Nano, body)
	if err != nil || at.Year() < 2000 {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// watchdogLastLogTime reads the newest entry of the inspection log. A line the
// watchdog did not write (a half-written one) is skipped, never guessed.
func watchdogLastLogTime() (time.Time, bool) {
	path := watchdogLogPath()
	if path == "" {
		return time.Time{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	lines := strings.Split(strings.TrimRight(string(body), "\r\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.SplitN(strings.TrimSpace(lines[i]), "\t", 2)
		if at, err := time.Parse(time.RFC3339, fields[0]); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

// SetWatchdogEnabled writes the policy and applies it at once, so the switch
// never waits for a restart. It returns the resulting status.
func (a *App) SetWatchdogEnabled(enabled bool) (WatchdogStatusView, error) {
	return setWatchdogEnabled(enabled)
}

// setWatchdogEnabled is the one implementation behind the CLI, the UI binding and
// the desktop directory, so all three can never disagree. It applies the entry; it
// does not decide whether the person opted out — that is setWatchdogOptOut's job.
func setWatchdogEnabled(enabled bool) (WatchdogStatusView, error) {
	policy := readWatchdogPolicy()
	policy.Enabled = enabled
	if err := writeWatchdogPolicy(policy); err != nil {
		view := watchdogStatusView()
		view.LastError = err.Error()
		return view, err
	}
	registered, err := applyWatchdogRegistration(enabled)
	if enabled {
		if err := writeWatchdogScript(); err != nil {
			slog.Warn("desktop watchdog: could not publish the script", "err", err)
		}
	} else if err := removeWatchdogScript(); err != nil {
		slog.Warn("desktop watchdog: could not remove the script", "err", err)
	}
	view := watchdogStatusView()
	if err != nil {
		view.LastError = err.Error()
		slog.Warn("desktop watchdog: registration failed", "enabled", enabled, "err", err)
		return view, err
	}
	view.Registered = registered
	slog.Info("desktop watchdog: policy applied", "enabled", enabled, "registered", registered)
	return view, nil
}

// applyWatchdogPolicyOnStart converges the OS entry on every start: a switch written
// by an older host, a hand-edited policy, or a version switch that would otherwise
// leave the scheduler pointing at nothing.
func applyWatchdogPolicyOnStart() {
	convergeWatchdogEntry()
}

// convergeWatchdogEntry is the single place that decides the entry's state, so every
// caller (start, the unattended switch) agrees instead of each owning a piece of it.
//
// It no longer follows the unattended switch (2026-10-05): the entry's job became
// "bring a host that died back up", which a phone reaching for that host needs
// whether or not anyone turned unattended on — and following the switch meant the
// entry was *unregistered* in the common case (nobody runs unattended), leaving
// nothing to do the waking. The switch still gates unattended driving in-app; an
// explicit `--watchdog-disable` is the only thing that keeps the entry off.
func convergeWatchdogEntry() {
	if !watchdogEntryWanted(readWatchdogPolicy()) {
		return
	}
	// Register/re-register and republish the script — and **write no policy**. `enabled`
	// belongs to the login item, so writing it here would turn on login autostart for every
	// machine with an install, which the login item's own contract forbids ("nothing here
	// registers itself on a machine that never asked").
	refreshWatchdogEntry()
}

// watchdogPolicyOptedOut reports an explicit "leave this machine's scheduler alone".
// It is the only thing that keeps the entry off, so a person who wants no OS entry
// cannot have one re-registered underneath them at every start.
func watchdogPolicyOptedOut(policy watchdogPolicy) bool {
	return policy.OptOut
}

// setWatchdogOptOut is the management entry point behind `--watchdog-disable` and
// `--watchdog-enable`: disabling records the opt-out *and* takes the entry away,
// enabling clears it and puts the entry back. It goes through syncWatchdogEntry (not
// the policy writer) so the login item's `enabled` is left exactly as the master switch
// set it.
func setWatchdogOptOut(optOut bool) (WatchdogStatusView, error) {
	policy := readWatchdogPolicy()
	policy.OptOut = optOut
	if err := writeWatchdogPolicy(policy); err != nil {
		view := watchdogStatusView()
		view.LastError = err.Error()
		return view, err
	}
	if err := syncWatchdogEntry(!optOut); err != nil {
		view := watchdogStatusView()
		view.LastError = err.Error()
		return view, err
	}
	return watchdogStatusView(), nil
}

// unattendedSwitchOnDisk reads the master switch straight from the config file, which is
// what the in-app driving decision needs: it runs before the engine exists, and the switch
// is the configured value, not the crash-degraded one. known=false (unreadable or invalid
// config) leaves the driving decision exactly as it is.
func unattendedSwitchOnDisk() (bool, bool) {
	root := config.MemoryUserDir()
	if root == "" {
		return false, false
	}
	body, err := os.ReadFile(filepath.Join(root, "heartbeat-tasks.json"))
	if err != nil {
		return false, false
	}
	var cfg struct {
		Unattended bool `json:"unattended"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return false, false
	}
	return cfg.Unattended, true
}

// syncWatchdogEntry registers or removes the OS entry that restores a host which
// died. Best effort by design — an OS that refuses the registration must not fail
// whatever asked for it, so the failure is logged instead.
func syncWatchdogEntry(enabled bool) error {
	if reason := watchdogSyncSkipReason(portableInstallRoot()); reason != "" {
		slog.Debug("desktop watchdog: not touching the OS entry", "reason", reason)
		return nil
	}
	if _, err := setWatchdogEnabled(enabled); err != nil {
		slog.Warn("desktop watchdog: the OS entry could not be applied", "enabled", enabled, "err", err)
		return err
	}
	slog.Info("desktop watchdog: OS entry applied", "enabled", enabled)
	return nil
}

// watchdogSyncSkipReason says why the switch must leave the OS entry alone, or
// "" when it owns it.
func watchdogSyncSkipReason(installRoot string) string {
	if strings.TrimSpace(os.Getenv("REASONIX_DEV")) != "" {
		return "this is a dev run"
	}
	if !watchdogSupportedPlatform() {
		return "this platform has no watchdog integration"
	}
	// Without a versioned install there is nothing to restore, and this also
	// keeps a unit-test run from touching the machine's real scheduler.
	if strings.TrimSpace(installRoot) == "" {
		return "no versioned install to watch"
	}
	return ""
}

// refreshWatchdogEntry re-applies registration and the desktop file without
// touching the policy, so a moved install converges on every start.
func refreshWatchdogEntry() {
	if _, err := applyWatchdogRegistration(true); err != nil {
		slog.Warn("desktop watchdog: could not refresh registration", "err", err)
	}
	if err := writeWatchdogScript(); err != nil {
		slog.Warn("desktop watchdog: could not refresh the desktop script", "err", err)
	}
}

// watchdogDirFunc is a seam for tests; production keeps the single watchdog file in
// the app's own directory, where a person can read, run or remove it by hand.
var watchdogDirFunc = defaultWatchdogDir

func watchdogDir() string { return watchdogDirFunc() }

func defaultWatchdogDir() string {
	// The state home, never a folder a person uses: the Desktop location this once
	// pointed at held their own files, and a migration deleted the whole directory
	// with them (2026-10-02). One dedicated directory holds the one file.
	root := config.MemoryUserDir()
	if strings.TrimSpace(root) == "" {
		return ""
	}
	return filepath.Join(root, "watchdog")
}

func watchdogScriptPath() string {
	dir := watchdogDir()
	if dir == "" {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(dir, "watchdog.sh")
	}
	return filepath.Join(dir, "watchdog.cmd")
}

// watchdogScriptMarker is in every body this file publishes, so removal can tell
// this app's script from anything else parked at the same path.
const watchdogScriptMarker = "REASONIX_WATCHDOG="

// watchdogOwnedDir reports whether dir is this app's own directory: strictly inside
// the state home, so nothing here can publish into or delete from a folder a person
// uses. The Desktop one the watchdog once sat in was deleted whole, with the 106
// files of theirs that were in it (2026-10-02).
func watchdogOwnedDir(dir string) bool {
	root := config.MemoryUserDir()
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(root) == "" {
		return false
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || filepath.IsAbs(rel) || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// watchdogDirHoldsNothingForeign is true while the directory is missing or holds
// only the script this app publishes: a directory with someone else's files in it
// is not ours to write into.
func watchdogDirHoldsNothingForeign(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return os.IsNotExist(err)
	}
	mine := filepath.Base(watchdogScriptPath())
	for _, entry := range entries {
		if entry.Name() != mine {
			return false
		}
	}
	return true
}

// writeWatchdogScript publishes the single watchdog file: one command a person
// can read, run or delete. It launches the stable launcher, which resolves the
// active version on every run, so no version is ever pinned here.
func writeWatchdogScript() error {
	path := watchdogScriptPath()
	if path == "" {
		return errors.New("no home directory for the watchdog script")
	}
	dir := watchdogDir()
	if !watchdogOwnedDir(dir) {
		return fmt.Errorf("refusing to publish the watchdog script outside the state directory: %s", dir)
	}
	if !watchdogDirHoldsNothingForeign(dir) {
		return fmt.Errorf("refusing to publish the watchdog script into a directory that holds other files: %s", dir)
	}
	root := portableInstallRoot()
	if root == "" {
		return errors.New("no versioned install to watch")
	}
	launcher, err := installlayout.StableRelaunchPath(root)
	if err != nil || strings.TrimSpace(launcher) == "" {
		return errors.New("no stable launcher to point the watchdog at")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(watchdogScriptBody(launcher)), 0o644)
}

// watchdogScriptBody is the script the scheduler runs: the stable launcher, with
// the watchdog mode handed over in the environment, so no version is pinned here.
func watchdogScriptBody(launcher string) string {
	if runtime.GOOS == "darwin" {
		return strings.Join([]string{
			"#!/bin/sh",
			"# Reasonix OS watchdog - the stable launcher resolves the active version,",
			"# so a new build is picked up without touching this file.",
			"REASONIX_WATCHDOG=1",
			`exec "` + launcher + `"`,
			"",
		}, "\n")
	}
	return strings.Join([]string{
		"@echo off",
		"rem Reasonix OS watchdog - the stable launcher resolves the active version,",
		"rem so a new build is picked up without touching this file. Run this by",
		"rem hand to check once, or delete it (and disable the watchdog) to stop it.",
		`set "REASONIX_WATCHDOG=1"`,
		`"` + launcher + `"`,
		"",
	}, "\r\n")
}

// removeWatchdogScript takes back only the file this app published: a directory, or
// a file without its marker, is left alone — the migration that deleted the Desktop
// directory whole is why removal is this narrow (2026-10-02).
func removeWatchdogScript() error {
	path := watchdogScriptPath()
	if path == "" {
		return nil
	}
	if !watchdogOwnedDir(watchdogDir()) {
		return fmt.Errorf("refusing to remove anything outside the state directory: %s", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove %s: it is not a regular file", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(body), watchdogScriptMarker) {
		return fmt.Errorf("refusing to remove %s: it is not the watchdog script", path)
	}
	return os.Remove(path)
}

// watchdogLogPath keeps the inspection trail in the host's own log directory so
// the desktop directory stays a single file.
func watchdogLogPath() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "desktop-watchdog.log")
}

func writeWatchdogLogLine(action, reason, detail string) {
	path := watchdogLogPath()
	if path == "" {
		return
	}
	line := time.Now().UTC().Format(time.RFC3339) + "\t" + action + "\t" + reason
	if strings.TrimSpace(detail) != "" {
		line += "\t" + detail
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}
