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
	// Watchdog defaults to on whenever the policy is enabled; false opts out
	// while keeping the login item.
	Watchdog *bool `json:"watchdog,omitempty"`
}

func watchdogPolicyPath() string {
	root := config.MemoryUserDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "desktop-autostart.json")
}

func watchdogPolicyEnabled(policy watchdogPolicy) bool {
	return policy.Enabled && (policy.Watchdog == nil || *policy.Watchdog)
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
// the upper bound on how long an unattended host can stay down.
const watchdogIntervalMinutes = 5

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
			// The task points at the script, never at a version: the script runs the
			// stable launcher, which resolves the active version on every run.
			//
			// Registration goes through PowerShell rather than `schtasks /Create`:
			// schtasks' trigger plus Windows' default conditions left the task
			// registered and never run on a real machine (2026-10-02), which the
			// status view cannot distinguish from a working watchdog.
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
}

// WatchdogStatus reports the policy, the machine, and the binary it would run.
func (a *App) WatchdogStatus() WatchdogStatusView {
	return watchdogStatusView()
}

func watchdogStatusView() WatchdogStatusView {
	policy := readWatchdogPolicy()
	view := WatchdogStatusView{
		Supported:  watchdogSupportedPlatform(),
		Policy:     watchdogPolicyEnabled(policy),
		Platform:   runtime.GOOS,
		EntryPoint: watchdogEntryPoint(),
	}
	if !view.Supported {
		view.Note = "this platform has no watchdog integration"
		return view
	}
	view.Registered = watchdogRegistrationPresent()
	return view
}

// SetWatchdogEnabled writes the policy and applies it at once, so the switch
// never waits for a restart. It returns the resulting status.
func (a *App) SetWatchdogEnabled(enabled bool) (WatchdogStatusView, error) {
	return setWatchdogEnabled(enabled)
}

// setWatchdogEnabled is the one implementation behind the CLI, the UI binding and
// the desktop directory, so all three can never disagree.
func setWatchdogEnabled(enabled bool) (WatchdogStatusView, error) {
	policy := readWatchdogPolicy()
	policy.Enabled = enabled
	policy.Watchdog = &enabled
	if err := writeWatchdogPolicy(policy); err != nil {
		view := watchdogStatusView()
		view.LastError = err.Error()
		return view, err
	}
	registered, err := applyWatchdogRegistration(enabled)
	if enabled {
		_ = writeWatchdogScript()
	} else {
		_ = removeWatchdogScript()
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

// applyWatchdogPolicyOnStart aligns the OS entry with the master switch on every
// start, so the entry follows the one gate a person touches: a switch written by
// an older host, or a policy edited by hand, converges here, and a version switch
// cannot leave the scheduler pointing at nothing.
func applyWatchdogPolicyOnStart() {
	unattended, known := unattendedSwitchOnDisk()
	if !known {
		return
	}
	if watchdogPolicyEnabled(readWatchdogPolicy()) != unattended {
		_ = syncWatchdogWithUnattended(unattended)
		return
	}
	if unattended {
		refreshWatchdogEntry()
	}
}

// unattendedSwitchOnDisk reads the master switch straight from the config file,
// which is all this needs: it runs before the engine exists, and the switch is
// the configured value, not the crash-degraded one. known=false (unreadable or
// invalid config) leaves the OS entry exactly as it is.
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

// syncWatchdogWithUnattended makes the master switch the only command a person
// has to remember: turning unattended on registers the OS entry, turning it off
// takes it away. Best effort by design — an OS that refuses the registration
// must not fail the switch itself, so the failure is logged instead.
func syncWatchdogWithUnattended(unattended bool) error {
	if reason := watchdogSyncSkipReason(portableInstallRoot()); reason != "" {
		slog.Debug("desktop watchdog: not following the unattended switch", "reason", reason)
		return nil
	}
	if _, err := setWatchdogEnabled(unattended); err != nil {
		slog.Warn("desktop watchdog: the unattended switch could not apply it", "unattended", unattended, "err", err)
		return err
	}
	slog.Info("desktop watchdog: followed the unattended switch", "unattended", unattended)
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

// watchdogDirFunc is a seam for tests; production keeps the single watchdog file
// on the desktop so it can be read, run and removed by hand.
var watchdogDirFunc = defaultWatchdogDir

func watchdogDir() string { return watchdogDirFunc() }

func defaultWatchdogDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "Desktop", "$")
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

// writeWatchdogScript publishes the single watchdog file: one command a person
// can read, run or delete. It launches the stable launcher, which resolves the
// active version on every run, so no version is ever pinned here.
func writeWatchdogScript() error {
	path := watchdogScriptPath()
	if path == "" {
		return errors.New("no home directory for the watchdog script")
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

func removeWatchdogScript() error {
	path := watchdogScriptPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
