package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestWatchdogOnlyRestoresAFreshUnattendedCrash(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-2 * time.Minute)
	stale := now.Add(-48 * time.Hour)
	cases := []struct {
		name    string
		outcome hostStateOutcome
		started time.Time
		exe     string
		want    bool
	}{
		{"clean exit leaves nothing to restore", hostStateOutcome{}, fresh, "/v/desktop.exe", false},
		{"an attended marker is not restored", hostStateOutcome{Seen: true, Dead: true, Unattended: false}, fresh, "/v/desktop.exe", false},
		{"a live host is left alone", hostStateOutcome{Seen: true, Dead: false, Unattended: true}, fresh, "/v/desktop.exe", false},
		{"a crashed unattended host is restored", hostStateOutcome{Seen: true, Dead: true, Unattended: true}, fresh, "/v/desktop.exe", true},
		{"an old crash is not resurrected", hostStateOutcome{Seen: true, Dead: true, Unattended: true}, stale, "/v/desktop.exe", false},
		{"no binary means nothing to launch", hostStateOutcome{Seen: true, Dead: true, Unattended: true}, fresh, "  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := watchdogDecide(tc.outcome, tc.started, now, tc.exe)
			if got.Launch != tc.want {
				t.Fatalf("launch = %v (%s), want %v", got.Launch, got.Reason, tc.want)
			}
			if got.Reason == "" {
				t.Fatal("every decision must explain itself in the log")
			}
		})
	}
}

func TestWatchdogFlagIsRecognisedExactly(t *testing.T) {
	if !hasWatchdogFlag([]string{"--watchdog"}) {
		t.Fatal("--watchdog must be recognised")
	}
	if !hasWatchdogFlag([]string{"--some-other", " --watchdog "}) {
		t.Fatal("surrounding whitespace comes from shell quoting and must not matter")
	}
	if hasWatchdogFlag([]string{"--host-rpc"}) {
		t.Fatal("an unrelated invocation must not become a watchdog run")
	}
	if hasWatchdogFlag(nil) {
		t.Fatal("no arguments is not a watchdog run")
	}
}

func TestWatchdogNeverOwnsAnOrdinaryLaunch(t *testing.T) {
	if handled, code := maybeRunDesktopWatchdog([]string{"--host-rpc"}); handled || code != 0 {
		t.Fatalf("handled = %v, code = %d: an ordinary launch must fall through", handled, code)
	}
}

func TestWatchdogModeRecognisesTheManagementCommands(t *testing.T) {
	cases := []struct {
		arg  string
		want string
	}{
		{"--watchdog", "--watchdog"},
		{"--watchdog-status", "--watchdog-status"},
		{"--watchdog-enable", "--watchdog-enable"},
		{"--watchdog-disable", "--watchdog-disable"},
		{"--other", ""},
	}
	for _, tc := range cases {
		if got := watchdogModeFromArgs([]string{tc.arg}); got != tc.want {
			t.Fatalf("mode(%q) = %q, want %q", tc.arg, got, tc.want)
		}
	}
}

func TestWatchdogScriptBodyRunsTheStableLauncher(t *testing.T) {
	body := watchdogScriptBody(`C:\rx\reasonix-launcher.exe`)
	if !strings.Contains(body, `"C:\rx\reasonix-launcher.exe"`) {
		t.Fatalf("the script must run the stable launcher, got:\n%s", body)
	}
	if strings.Contains(body, "versions") {
		t.Fatal("the script must not pin a version: the launcher resolves it on every run")
	}
	if strings.Contains(body, "--watchdog") {
		t.Fatal("the launcher takes no watchdog flag: the mode travels in the environment")
	}
	if !strings.Contains(body, "REASONIX_WATCHDOG") {
		t.Fatalf("the script must set the watchdog mode variable, got:\n%s", body)
	}
}

func TestWatchdogEnvSelectsTheModeWithoutArguments(t *testing.T) {
	t.Setenv(watchdogEnvFlag, "1")
	if got := watchdogModeFromArgs(nil); got != "" {
		t.Fatalf("no arguments alone must not be a watchdog run, got %q", got)
	}
	// maybeRunDesktopWatchdog owns the invocation once the environment asks for it.
	if handled, _ := maybeRunDesktopWatchdog(nil); !handled {
		t.Fatal("REASONIX_WATCHDOG must select the watchdog mode")
	}
}

func TestWatchdogEnvNeverReachesTheChild(t *testing.T) {
	base := []string{"PATH=C:\\bin", "reasonix_watchdog=1", "REASONIX_WATCHDOG=1", "OTHER=1"}
	got := watchdogEnvWithoutChildFlag(base)
	for _, entry := range got {
		if strings.HasPrefix(strings.ToUpper(entry), "REASONIX_WATCHDOG=") {
			t.Fatalf("the child inherited the watchdog marker: %v", got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("filtered env = %v, want the two unrelated entries", got)
	}
}

func TestWatchdogPolicyDefaultsToOnWithThePolicy(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name   string
		policy watchdogPolicy
		want   bool
	}{
		{"enabled without an opinion", watchdogPolicy{Enabled: true}, true},
		{"explicitly on", watchdogPolicy{Enabled: true, Watchdog: &on}, true},
		{"opted out", watchdogPolicy{Enabled: true, Watchdog: &off}, false},
		{"policy disabled", watchdogPolicy{Enabled: false, Watchdog: &on}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := watchdogPolicyEnabled(tc.policy); got != tc.want {
				t.Fatalf("enabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWatchdogScriptIsOneFileThatComesAndGoes(t *testing.T) {
	dir := t.TempDir()
	previous := watchdogDirFunc
	watchdogDirFunc = func() string { return dir }
	t.Cleanup(func() { watchdogDirFunc = previous })

	// Without an active version there is nothing to point the script at, so it
	// must refuse rather than publish a file that cannot run.
	if err := writeWatchdogScript(); err == nil {
		t.Fatal("no active desktop binary means no script to write")
	}
	if err := removeWatchdogScript(); err != nil {
		t.Fatalf("removing an absent script must be a no-op: %v", err)
	}
	if err := os.WriteFile(watchdogScriptPath(), []byte("@echo off\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := removeWatchdogScript(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(watchdogScriptPath()); !os.IsNotExist(err) {
		t.Fatalf("the script must be gone, got %v", err)
	}
}
