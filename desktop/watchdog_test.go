package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/installlayout"
)

func TestWatchdogRestoresAHostThatDiedWithoutClearingItsMarker(t *testing.T) {
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
		// 这一条是本轮的要害：替手机去够一台已经不在了的桌面端时，那次运行是不是"无人值守"
		// 无关紧要 —— 印记还在就说明它没干净退出过。
		{"an attended crash is restored too", hostStateOutcome{Seen: true, Dead: true, Unattended: false}, fresh, "/v/desktop.exe", true},
		{"a live host is left alone", hostStateOutcome{Seen: true, Dead: false, Unattended: true}, fresh, "/v/desktop.exe", false},
		{"a crashed unattended host is restored", hostStateOutcome{Seen: true, Dead: true, Unattended: true}, fresh, "/v/desktop.exe", true},
		{"a crash loop is left to its window", hostStateOutcome{Seen: true, Dead: true, UncleanStreak: hostCrashStreakLimit}, fresh, "/v/desktop.exe", false},
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

// The entry's own policy: on unless someone opted out. `enabled` deliberately does not
// enter into it — that field is the login item's, and a machine whose switch is off must
// still get the entry that brings a dead host back, without also getting login autostart.
func TestWatchdogEntryIsWantedUnlessOptedOut(t *testing.T) {
	cases := []struct {
		name   string
		policy watchdogPolicy
		want   bool
	}{
		{"no policy file at all", watchdogPolicy{}, true},
		{"the switch is off", watchdogPolicy{Enabled: false}, true},
		{"the switch is on", watchdogPolicy{Enabled: true}, true},
		{"opted out", watchdogPolicy{Enabled: true, OptOut: true}, false},
		{"opted out while the switch is off", watchdogPolicy{Enabled: false, OptOut: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := watchdogEntryWanted(tc.policy); got != tc.want {
				t.Fatalf("entryWanted = %v, want %v", got, tc.want)
			}
		})
	}
}

// A policy file written by the old behaviour — where turning the unattended switch off
// recorded `"watchdog":false` — must not read as an opt-out. It meant "I am not running
// unattended", not "leave my scheduler alone", and treating it as the latter would
// silently switch off the very entry that brings a dead host back (2026-10-05).
func TestLegacyPolicyFileDoesNotCountAsAnOptOut(t *testing.T) {
	harness := t.TempDir()
	t.Setenv("REASONIX_HOME", harness)
	t.Setenv("REASONIX_STATE_HOME", harness)
	if got := config.MemoryUserDir(); got != harness {
		t.Fatalf("the harness home must be the state directory: %q != %q", got, harness)
	}
	body := `{"schemaVersion":1,"enabled":false,"watchdog":false}`
	if err := os.WriteFile(watchdogPolicyPath(), []byte(body), 0o600); err != nil {
		t.Fatalf("seed the legacy policy: %v", err)
	}
	policy := readWatchdogPolicy()
	if watchdogPolicyOptedOut(policy) {
		t.Fatalf("a legacy watchdog:false must not opt out, got %+v", policy)
	}
	if !watchdogEntryWanted(policy) {
		t.Fatalf("the entry must still be wanted, got %+v", policy)
	}
}

func TestWatchdogScriptIsOneFileThatComesAndGoes(t *testing.T) {
	// The file only ever lives in the app's own directory, so the seam is pointed at
	// the state home the ownership guard checks against.
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	dir := filepath.Join(home, "watchdog")
	previous, previousRoot := watchdogDirFunc, portableInstallRootFunc
	watchdogDirFunc = func() string { return dir }
	portableInstallRootFunc = func() string { return "" }
	t.Cleanup(func() { watchdogDirFunc, portableInstallRootFunc = previous, previousRoot })

	// Without an active version there is nothing to point the script at, so it
	// must refuse rather than publish a file that cannot run.
	if err := writeWatchdogScript(); err == nil {
		t.Fatal("no active desktop binary means no script to write")
	}
	if err := removeWatchdogScript(); err != nil {
		t.Fatalf("removing an absent script must be a no-op: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seed the directory: %v", err)
	}
	if err := os.WriteFile(watchdogScriptPath(), []byte(watchdogScriptBody("launcher")), 0o644); err != nil {
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

// The OS entry's job is now "bring a host that died back up", which a phone needs
// whether or not anyone runs unattended — so it no longer follows that switch
// (2026-10-05). This pins the new contract: the entry is registered even with the
// switch off, and only an explicit opt-out takes it away for good.
func TestWatchdogEntryIsRegisteredWhateverTheUnattendedSwitchSays(t *testing.T) {
	if !watchdogSupportedPlatform() {
		t.Skip("this platform has no OS entry to register")
	}
	calls, home := watchdogTestHarness(t)

	// A start with the switch OFF still registers: that is the case a dead host has to
	// come back from, and the switch has no say in it. It must not write `enabled` on the
	// way — that field belongs to the login item, and writing it here would turn on login
	// autostart for a machine whose switch is off, which the login item's own contract
	// forbids ("nothing here registers itself on a machine that never asked").
	writeHeartbeatSwitch(t, home, false)
	applyWatchdogPolicyOnStart()
	if !calls.created || !watchdogEntryWanted(readWatchdogPolicy()) {
		t.Fatal("a start with the switch off must still register the entry")
	}
	if policy := readWatchdogPolicy(); policy.Enabled {
		t.Fatalf("converging the entry must not touch the login item's enabled, got %+v", policy)
	}
	if _, err := os.Stat(watchdogScriptPath()); err != nil {
		t.Fatalf("a registered entry needs its published script: %v", err)
	}
	if !strings.HasPrefix(watchdogScriptPath(), home) {
		t.Fatalf("the desktop file must live in the harness home, got %s", watchdogScriptPath())
	}

	// Flipping the switch on converges it again — idempotent, and it does not touch the
	// opt-out either way.
	writeHeartbeatSwitch(t, home, true)
	calls.created = false
	applyWatchdogPolicyOnStart()
	if !calls.created {
		t.Fatal("a start with the switch on must keep the entry registered")
	}

	// Only an explicit opt-out takes it away.
	calls.deleted = false
	if _, err := setWatchdogOptOut(true); err != nil {
		t.Fatalf("an explicit disable must take the entry away: %v", err)
	}
	if !calls.deleted || watchdogEntryWanted(readWatchdogPolicy()) {
		t.Fatal("opting out must leave a disabled policy and no entry")
	}
	if _, err := os.Stat(watchdogScriptPath()); !os.IsNotExist(err) {
		t.Fatalf("opting out must remove the published script, got %v", err)
	}
	calls.created = false
	applyWatchdogPolicyOnStart()
	if calls.created {
		t.Fatal("a start must not re-register an entry the person explicitly disabled")
	}

	// Enabling clears the opt-out and puts the entry back.
	if _, err := setWatchdogOptOut(false); err != nil {
		t.Fatalf("clearing the opt-out must work: %v", err)
	}
	if !calls.created || !watchdogEntryWanted(readWatchdogPolicy()) {
		t.Fatal("clearing the opt-out must put the entry back")
	}
}

func TestWatchdogSyncSkipsWithoutAnInstallToWatch(t *testing.T) {
	t.Setenv("REASONIX_DEV", "")
	if watchdogSyncSkipReason("") == "" || watchdogSyncSkipReason("   ") == "" {
		t.Fatal("no versioned install must keep the entry off the machine's scheduler")
	}
	if !watchdogSupportedPlatform() {
		return
	}
	root := t.TempDir()
	if reason := watchdogSyncSkipReason(root); reason != "" {
		t.Fatalf("a versioned install must let the entry be registered, got %q", reason)
	}
	t.Setenv("REASONIX_DEV", "1")
	if watchdogSyncSkipReason(root) == "" {
		t.Fatal("a dev run must never touch the machine's scheduler")
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
	if err := syncWatchdogEntry(true); err == nil {
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
	if err := syncWatchdogEntry(true); err != nil {
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
	if err := syncWatchdogEntry(true); err != nil {
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

// The directory the watchdog publishes into must never be one a person uses: the
// Desktop location it once pointed at was deleted whole, with the 106 files of theirs
// that were in it (2026-10-02). Outside the state home both calls must refuse.
func TestWatchdogRefusesToTouchADirectoryItDoesNotOwn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	theirs := t.TempDir()
	previous := watchdogDirFunc
	watchdogDirFunc = func() string { return theirs }
	t.Cleanup(func() { watchdogDirFunc = previous })

	theirFile := filepath.Join(theirs, "Adobe Acrobat DC.lnk")
	theirDir := filepath.Join(theirs, "keep")
	if err := os.MkdirAll(theirDir, 0o755); err != nil {
		t.Fatalf("seed their directory: %v", err)
	}
	if err := os.WriteFile(theirFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed their file: %v", err)
	}
	if err := os.WriteFile(watchdogScriptPath(), []byte(watchdogScriptBody("launcher")), 0o644); err != nil {
		t.Fatalf("seed a script-shaped file of theirs: %v", err)
	}

	if err := writeWatchdogScript(); err == nil {
		t.Fatal("publishing outside the state directory must refuse")
	}
	if err := removeWatchdogScript(); err == nil {
		t.Fatal("removing outside the state directory must refuse")
	}
	for _, path := range []string{theirFile, theirDir, watchdogScriptPath()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s must survive: %v", path, err)
		}
	}
}

// A directory holding someone else's files is not ours to publish into, even inside
// the state home — that is what made the old Desktop location dangerous.
func TestWatchdogRefusesToPublishIntoADirectoryThatHoldsOtherFiles(t *testing.T) {
	_, home := watchdogTestHarness(t)
	dir := filepath.Join(home, "watchdog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seed the directory: %v", err)
	}
	theirFile := filepath.Join(dir, "their-notes.txt")
	if err := os.WriteFile(theirFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed their file: %v", err)
	}

	if err := writeWatchdogScript(); err == nil {
		t.Fatal("a directory holding someone else's files is not ours to write into")
	}
	if err := removeWatchdogScript(); err != nil {
		t.Fatalf("an absent script is still a no-op: %v", err)
	}
	if _, err := os.Stat(theirFile); err != nil {
		t.Fatalf("their file must survive: %v", err)
	}
}

// Removal may take only the file this app published: a directory at that path, or a
// file someone else put there, stays exactly where it is.
func TestWatchdogRemovesOnlyItsOwnScript(t *testing.T) {
	watchdogTestHarness(t)
	path := watchdogScriptPath()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("seed a directory at the script path: %v", err)
	}
	if err := removeWatchdogScript(); err == nil {
		t.Fatal("a directory at the script path is not ours to remove")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the directory must survive: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("clear the directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("rem a file the person left at this path"), 0o644); err != nil {
		t.Fatalf("seed their file: %v", err)
	}
	if err := removeWatchdogScript(); err == nil {
		t.Fatal("a file that is not the watchdog script is not ours to remove")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("their file must survive: %v", err)
	}
}

// "Registered" is what the machine says it was told, never what it did: a task
// Windows accepts and never triggers reads exactly like a working one.
func TestWatchdogRunTimeFromTaskInfoIsHonestAboutNever(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"the scheduler's answer", "2026-10-02T15:54:54.0000000Z\r\n", "2026-10-02T15:54:54Z"},
		{"a local answer is converted to UTC", "2026-10-02T15:54:54.0000000+08:00", "2026-10-02T07:54:54Z"},
		{"a redirected stream may carry a BOM", "\ufeff2026-10-02T15:54:54.0000000Z", "2026-10-02T15:54:54Z"},
		{"never ran (the Windows sentinel)", "1999-11-30T16:00:00.0000000Z", ""},
		{"the task is gone", "", ""},
		{"an error message is not a time", "Get-ScheduledTaskInfo : No MSFT_ScheduledTask objects found", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := watchdogRunTimeFromTaskInfo(tc.in); got != tc.want {
				t.Fatalf("watchdogRunTimeFromTaskInfo(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Where the scheduler cannot be asked, the inspection log is the record that the
// watchdog ran — and a line it did not finish writing is not a run time.
func TestWatchdogLastRunFallsBackToTheInspectionLog(t *testing.T) {
	_, _ = watchdogTestHarness(t)
	if _, ok := watchdogLastLogTime(); ok {
		t.Fatal("an absent log means nothing ran, not a made-up time")
	}
	writeWatchdogLogLine("skip", "no marker: the last run exited cleanly or never started", "marker: seen=false")
	body, err := os.ReadFile(watchdogLogPath())
	if err != nil {
		t.Fatalf("read the inspection log: %v", err)
	}
	// A tick killed mid-write leaves no timestamp: it must be skipped, not guessed.
	if err := os.WriteFile(watchdogLogPath(), append(body, []byte("2026-10-03T00:0")...), 0o600); err != nil {
		t.Fatalf("seed a half-written line: %v", err)
	}
	at, ok := watchdogLastLogTime()
	if !ok {
		t.Fatal("the last written tick is a run time")
	}
	if time.Since(at) > time.Hour {
		t.Fatalf("the newest usable line is the one just written, got %s", at)
	}
}

func TestWatchdogStatusReportsTheRunTimeTheSchedulerGives(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows keeps a run time next to the entry")
	}
	_, _ = watchdogTestHarness(t)
	watchdogPlatformRunner = func(name string, args ...string) ([]byte, error) {
		if !strings.Contains(strings.Join(args, " "), "Get-ScheduledTaskInfo") {
			return nil, nil // schtasks /Query: the entry is there
		}
		return []byte("2026-10-02T15:54:54.0000000Z\r\n"), nil
	}
	view := watchdogStatusView()
	if !view.Registered {
		t.Fatal("the harness's fake scheduler reports the entry as present")
	}
	if view.LastRunAt != "2026-10-02T15:54:54Z" {
		t.Fatalf("LastRunAt = %q, want the scheduler's answer", view.LastRunAt)
	}
	if !strings.Contains(watchdogStatusText(), "last run: 2026-10-02T15:54:54Z") {
		t.Fatalf("the CLI must show it too:\n%s", watchdogStatusText())
	}
}

func TestWatchdogStatusSaysNeverInsteadOfStayingSilent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows keeps a run time next to the entry")
	}
	_, _ = watchdogTestHarness(t)
	// The sentinel a task that never ran answers with.
	watchdogPlatformRunner = func(name string, args ...string) ([]byte, error) {
		return []byte("1999-11-30T16:00:00.0000000Z\r\n"), nil
	}
	view := watchdogStatusView()
	if !view.Registered || view.LastRunAt != "" {
		t.Fatalf("registered with no run must report no run, got %+v", view)
	}
	if !strings.Contains(watchdogStatusText(), "last run: never") {
		t.Fatalf("a registered entry that never ran must say so:\n%s", watchdogStatusText())
	}
}
