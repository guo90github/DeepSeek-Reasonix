// Desktop watchdog — the pre-shell mode an OS scheduler runs to bring the app
// back after a crash, restoring only what the launch marker says should be
// running and is not, so a deliberate quit is never undone.

package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"reasonix/internal/installlayout"
	"reasonix/internal/proc"
)

const (
	watchdogFlag    = "--watchdog"
	watchdogEnvFlag = "REASONIX_WATCHDOG"
	// watchdogFreshWindow bounds how old a crashed run may be and still be
	// restarted: a marker left by an old session must not resurrect the app.
	watchdogFreshWindow = 24 * time.Hour
)

type watchdogDecision struct {
	Launch bool
	Exe    string
	Reason string
}

// watchdogDecide is the whole policy: restart only a fresh, unattended run whose
// process is gone.
func watchdogDecide(outcome hostStateOutcome, startedAt, now time.Time, exe string) watchdogDecision {
	if !outcome.Seen {
		return watchdogDecision{Reason: "no marker: the last run exited cleanly or never started"}
	}
	if !outcome.Unattended {
		return watchdogDecision{Reason: "the marker does not ask for unattended"}
	}
	if !outcome.Dead {
		return watchdogDecision{Reason: "the recorded host is still running"}
	}
	if !startedAt.IsZero() && now.Sub(startedAt) > watchdogFreshWindow {
		return watchdogDecision{Reason: "the crash is older than the freshness window"}
	}
	if strings.TrimSpace(exe) == "" {
		return watchdogDecision{Reason: "no active desktop binary to launch"}
	}
	return watchdogDecision{Launch: true, Exe: exe, Reason: "restoring an unattended host that is gone"}
}

// watchdogDecideNow gathers what watchdogDecide needs: the marker, its start
// time, and the active version's own desktop binary — the single launch entry.
func watchdogDecideNow() watchdogDecision {
	record, _ := readHostState()
	outcome := hostStateBeforeLaunch()
	startedAt, _ := time.Parse(time.RFC3339Nano, record.StartedAt)
	exe := ""
	if root := portableInstallRoot(); root != "" {
		if path, err := installlayout.ActiveDesktopPath(root); err == nil {
			exe = path
		}
	}
	return watchdogDecide(outcome, startedAt, time.Now(), exe)
}

func hasWatchdogFlag(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == watchdogFlag {
			return true
		}
	}
	return false
}

// The management commands share the scheduler's entry point, so the OS entry can
// be inspected and changed even when the desktop itself cannot start.
const (
	watchdogStatusFlag  = "--watchdog-status"
	watchdogEnableFlag  = "--watchdog-enable"
	watchdogDisableFlag = "--watchdog-disable"
)

func watchdogModeFromArgs(args []string) string {
	for _, arg := range args {
		switch trimmed := strings.TrimSpace(arg); trimmed {
		case watchdogFlag, watchdogStatusFlag, watchdogEnableFlag, watchdogDisableFlag:
			return trimmed
		}
	}
	return ""
}

// maybeRunDesktopWatchdog runs the watchdog modes and reports whether they owned
// the invocation. The scheduler calls --watchdog every few minutes, where the
// answer is always "launch the active desktop" or "nothing to do" — never a
// shell boot, which would fight the single-instance lock.
func maybeRunDesktopWatchdog(args []string) (bool, int) {
	mode := watchdogModeFromArgs(args)
	if mode == "" && strings.TrimSpace(os.Getenv(watchdogEnvFlag)) != "" {
		mode = watchdogFlag
	}
	if mode == "" {
		return false, 0
	}
	if os.Getenv("REASONIX_DEV") != "" {
		return true, 0
	}
	switch mode {
	case watchdogEnableFlag, watchdogDisableFlag:
		enabled := mode == watchdogEnableFlag
		if _, err := setWatchdogEnabled(enabled); err != nil {
			fmt.Fprintf(os.Stderr, "reasonix-desktop: %v\n", err)
			fmt.Print(watchdogStatusText())
			return true, 1
		}
		fmt.Print(watchdogStatusText())
		return true, 0
	case watchdogStatusFlag:
		fmt.Print(watchdogStatusText())
		return true, 0
	}
	decision := watchdogDecideNow()
	if !decision.Launch {
		slog.Debug("desktop watchdog: nothing to do", "reason", decision.Reason)
		writeWatchdogLogLine("skip", decision.Reason, "")
		return true, 0
	}
	cmd := proc.VisibleCommand(decision.Exe)
	// The child boots normally: it must not re-enter the watchdog mode.
	cmd.Env = watchdogEnvWithoutChildFlag(os.Environ())
	if err := cmd.Start(); err != nil {
		slog.Error("desktop watchdog: could not launch", "exe", decision.Exe, "err", err)
		writeWatchdogLogLine("failed", decision.Reason, err.Error())
		return true, 1
	}
	slog.Info("desktop watchdog: relaunched the desktop", "exe", decision.Exe)
	writeWatchdogLogLine("launched", decision.Reason, decision.Exe)
	return true, 0
}

// watchdogEnvWithoutChildFlag keeps the watchdog's own marker out of the child's
// environment, so the process it starts boots the desktop instead of looping.
func watchdogEnvWithoutChildFlag(base []string) []string {
	out := make([]string, 0, len(base))
	for _, entry := range base {
		if strings.HasPrefix(strings.ToUpper(entry), watchdogEnvFlag+"=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// watchdogStatusText is what the CLI and the desktop directory show.
func watchdogStatusText() string {
	policy := readWatchdogPolicy()
	registered := watchdogRegistrationPresent()
	state := "not registered"
	if registered {
		state = "registered"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "OS watchdog: %s\n", state)
	fmt.Fprintf(&b, "policy: enabled=%v watchdog=%v\n", policy.Enabled, policy.Watchdog == nil || *policy.Watchdog)
	fmt.Fprintf(&b, "platform: %s\n", runtime.GOOS)
	if exe := watchdogEntryPoint(); exe != "" {
		fmt.Fprintf(&b, "entry point: %s\n", exe)
	}
	fmt.Fprintf(&b, "directory: %s\n", watchdogDir())
	return b.String()
}
