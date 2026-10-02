package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/installlayout"
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

// watchdogRunnerCalls is the fake scheduler: it records the two commands the
// watchdog management issues and can refuse them like a locked-down machine.
type watchdogRunnerCalls struct {
	created bool
	deleted bool
	fail    bool
	// commands records every scheduler command in order, so a test can assert what the
	// machine was actually told rather than only the last thing it was asked.
	commands []string
}

func (c *watchdogRunnerCalls) run(name string, args ...string) ([]byte, error) {
	if c.fail {
		return []byte("access denied"), errors.New("exit status 1")
	}
	joined := strings.Join(append([]string{name}, args...), " ")
	c.commands = append(c.commands, joined)
	if strings.Contains(joined, "/Create") || strings.Contains(joined, "Register-ScheduledTask") {
		c.created = true
	}
	if strings.Contains(joined, "/Delete") || strings.Contains(joined, "Unregister-ScheduledTask") {
		c.deleted = true
	}
	return nil, nil
}

// registrationCommand is the command that creates the OS entry, or "" if none did.
func (c *watchdogRunnerCalls) registrationCommand() string {
	for _, command := range c.commands {
		if strings.Contains(command, "Register-ScheduledTask") || strings.Contains(command, "/Create") {
			return command
		}
	}
	return ""
}

// watchdogTestHarness points every OS-facing seam at temp paths, so a test can
// never reach the machine's own scheduler, state directory or desktop.
func watchdogTestHarness(t *testing.T) (*watchdogRunnerCalls, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	t.Setenv("REASONIX_DEV", "")
	// The switch and the policy both live in the user state directory, so the
	// harness has to own it before anything reads or writes it.
	if got := config.MemoryUserDir(); got != home {
		t.Fatalf("the harness home must be the state directory: %q != %q", got, home)
	}

	root := t.TempDir()
	versionDir := filepath.Join(root, "versions", "v0.0.0-dev.1")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("seed the version directory: %v", err)
	}
	for _, seed := range []struct{ dir, name string }{
		{versionDir, installlayout.DesktopBinaryName()},
		{root, installlayout.LauncherBinaryName()},
	} {
		if strings.TrimSpace(seed.name) == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(seed.dir, seed.name), []byte("x"), 0o755); err != nil {
			t.Fatalf("seed %s: %v", filepath.Join(seed.dir, seed.name), err)
		}
	}
	pointer := `{"schemaVersion":1,"activeVersion":"v0.0.0-dev.1","activeDir":"versions/v0.0.0-dev.1"}`
	if err := os.WriteFile(filepath.Join(root, "current.json"), []byte(pointer), 0o644); err != nil {
		t.Fatalf("seed current.json: %v", err)
	}

	calls := &watchdogRunnerCalls{}
	previousRoot, previousDir, previousRunner := portableInstallRootFunc, watchdogDirFunc, watchdogPlatformRunner
	portableInstallRootFunc = func() string { return root }
	watchdogDirFunc = func() string { return filepath.Join(home, "watchdog") }
	watchdogPlatformRunner = calls.run
	t.Cleanup(func() {
		portableInstallRootFunc = previousRoot
		watchdogDirFunc = previousDir
		watchdogPlatformRunner = previousRunner
	})
	return calls, home
}

// The switch on the UI and the CLI must report the same binary the script runs,
// or the two silently disagree about what the OS entry starts.
func TestWatchdogStatusEntryPointIsWhatTheScriptRuns(t *testing.T) {
	_, _ = watchdogTestHarness(t)
	if err := writeWatchdogScript(); err != nil {
		t.Fatalf("write the watchdog script: %v", err)
	}
	body, err := os.ReadFile(watchdogScriptPath())
	if err != nil {
		t.Fatalf("read the watchdog script: %v", err)
	}
	entry := watchdogEntryPoint()
	if filepath.Base(entry) != installlayout.LauncherBinaryName() {
		t.Fatalf("the entry point must be the install root launcher, got %q", entry)
	}
	if !strings.Contains(string(body), entry) {
		t.Fatalf("the script does not run the reported entry point:\nscript=%q\nentry=%q", body, entry)
	}
}

func TestWatchdogFollowsTheMasterSwitchBothWays(t *testing.T) {
	if !watchdogSupportedPlatform() {
		t.Skip("this platform has no OS entry to register")
	}
	calls, home := watchdogTestHarness(t)

	if err := syncWatchdogWithUnattended(true); err != nil {
		t.Fatalf("turning the switch on must register the watchdog: %v", err)
	}
	if policy := readWatchdogPolicy(); !watchdogPolicyEnabled(policy) {
		t.Fatalf("the switch on must leave an enabled policy, got %+v", policy)
	}
	if !calls.created {
		t.Fatal("the switch on must register the OS entry")
	}
	if _, err := os.Stat(watchdogScriptPath()); err != nil {
		t.Fatalf("the switch on must publish the desktop file: %v", err)
	}
	if !strings.HasPrefix(watchdogScriptPath(), home) {
		t.Fatalf("the desktop file must live in the harness home, got %s", watchdogScriptPath())
	}

	calls.created = false
	if err := syncWatchdogWithUnattended(false); err != nil {
		t.Fatalf("turning the switch off must remove the entry: %v", err)
	}
	if policy := readWatchdogPolicy(); watchdogPolicyEnabled(policy) {
		t.Fatalf("the switch off must leave a disabled policy, got %+v", policy)
	}
	if !calls.deleted {
		t.Fatal("the switch off must unregister the OS entry")
	}
	if _, err := os.Stat(watchdogScriptPath()); !os.IsNotExist(err) {
		t.Fatalf("the switch off must remove the desktop file, got %v", err)
	}
}

func TestWatchdogSyncSkipsWithoutAnInstallToWatch(t *testing.T) {
	t.Setenv("REASONIX_DEV", "")
	if watchdogSyncSkipReason("") == "" || watchdogSyncSkipReason("   ") == "" {
		t.Fatal("no versioned install must keep the switch away from the OS entry")
	}
	if !watchdogSupportedPlatform() {
		return
	}
	root := t.TempDir()
	if reason := watchdogSyncSkipReason(root); reason != "" {
		t.Fatalf("a versioned install must let the switch own the entry, got %q", reason)
	}
	t.Setenv("REASONIX_DEV", "1")
	if watchdogSyncSkipReason(root) == "" {
		t.Fatal("a dev run must never touch the machine's scheduler")
	}
}

func TestWatchdogEntryFollowsTheSwitchOnStart(t *testing.T) {
	if !watchdogSupportedPlatform() {
		t.Skip("this platform has no OS entry to register")
	}
	calls, home := watchdogTestHarness(t)

	// A switch that is on while the stored policy is off converges at start.
	writeHeartbeatSwitch(t, home, true)
	applyWatchdogPolicyOnStart()
	if !calls.created || !watchdogPolicyEnabled(readWatchdogPolicy()) {
		t.Fatal("a start with the switch on must register the entry")
	}

	// And one that is off while the policy is on is taken away again.
	writeHeartbeatSwitch(t, home, false)
	calls.created, calls.deleted = false, false
	applyWatchdogPolicyOnStart()
	if !calls.deleted || watchdogPolicyEnabled(readWatchdogPolicy()) {
		t.Fatal("a start with the switch off must take the entry away")
	}
}

func writeHeartbeatSwitch(t *testing.T, home string, on bool) {
	t.Helper()
	body := fmt.Sprintf(`{"schemaVersion":2,"unattended":%t,"tasks":[]}`, on)
	if err := os.WriteFile(filepath.Join(home, "heartbeat-tasks.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("seed heartbeat-tasks.json: %v", err)
	}
}

func TestWatchdogRefusalIsReportedNotSwallowed(t *testing.T) {
	if !watchdogSupportedPlatform() {
		t.Skip("this platform has no OS entry to register")
	}
	calls, _ := watchdogTestHarness(t)
	calls.fail = true
	if err := syncWatchdogWithUnattended(true); err == nil {
		t.Fatal("a refused registration must be reported to the caller")
	}
}

// The registration has to produce a task that actually runs. Windows' defaults (no
// start on battery power, no catch-up for a missed run) left the entry registered and
// never run on a real machine, so the command is asserted rather than trusted.
func TestWatchdogRegistrationSetsTheConditionsThatMakeItRun(t *testing.T) {
	if !watchdogSupportedPlatform() {
		t.Skip("this platform has no OS entry to register")
	}
	calls, _ := watchdogTestHarness(t)
	if err := syncWatchdogWithUnattended(true); err != nil {
		t.Fatalf("register the watchdog: %v", err)
	}
	registration := calls.registrationCommand()
	if registration == "" {
		t.Fatal("nothing registered the OS entry")
	}
	for _, want := range []string{
		"New-ScheduledTaskAction",
		"Register-ScheduledTask",
		"-AllowStartIfOnBatteries",
		"-DontStopIfGoingOnBatteries",
		"-StartWhenAvailable",
		"-RepetitionInterval",
		watchdogTaskName,
		"watchdog.cmd",
	} {
		if !strings.Contains(registration, want) {
			t.Fatalf("the registration command is missing %q:\n%s", want, registration)
		}
	}
	if strings.Contains(registration, "schtasks") {
		t.Fatalf("the registration must not go back to schtasks and its default conditions:\n%s", registration)
	}
}

// The default location has to be a path the OS can actually create. `…\Desktop\$` is
// not: Windows folds it onto the Desktop, so the registered task names a script that
// does not exist and every run fails — while the status view still says "registered".
func TestDefaultWatchdogDirIsAPathWindowsCanCreate(t *testing.T) {
	dir := defaultWatchdogDir()
	if dir == "" {
		t.Skip("no user state directory on this platform")
	}
	if strings.Contains(dir, "$") {
		t.Fatalf("the watchdog file must not live at a $-named path Windows folds away: %q", dir)
	}
	if strings.EqualFold(filepath.Base(filepath.Dir(dir)), "Desktop") {
		t.Fatalf("the watchdog file must not live on the Desktop: %q", dir)
	}
	if !strings.HasSuffix(strings.ReplaceAll(dir, `\`, "/"), "/watchdog") {
		t.Fatalf("the watchdog file needs its own directory: %q", dir)
	}
}

// What the scheduler is told to run must be the file the app publishes, or the entry is
// registered against something nobody wrote.
func TestWatchdogRegistrationRunsTheScriptItPublishes(t *testing.T) {
	if !watchdogSupportedPlatform() {
		t.Skip("this platform has no OS entry to register")
	}
	calls, _ := watchdogTestHarness(t)
	if err := syncWatchdogWithUnattended(true); err != nil {
		t.Fatalf("register the watchdog: %v", err)
	}
	script := watchdogScriptPath()
	if script == "" {
		t.Fatal("the harness must publish a script path")
	}
	registration := calls.registrationCommand()
	if !strings.Contains(registration, script) {
		t.Fatalf("the registration does not run the published script %q:\n%s", script, registration)
	}
}
