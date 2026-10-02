package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldResumeUnattendedSession(t *testing.T) {
	cases := []struct {
		name        string
		interrupted bool
		unattended  bool
		goal        string
		already     bool
		want        bool
	}{
		{name: "interrupted", interrupted: true, unattended: true, goal: "把 docs 做完", want: true},
		{name: "clean exit interrupted nothing", interrupted: false, unattended: true, goal: "把 docs 做完"},
		{name: "switch off", interrupted: true, unattended: false, goal: "把 docs 做完"},
		{name: "no contract", interrupted: true, unattended: true, goal: "   "},
		{name: "already queued this run", interrupted: true, unattended: true, goal: "把 docs 做完", already: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldResumeUnattendedSession(tc.interrupted, tc.unattended, tc.goal, tc.already); got != tc.want {
				t.Fatalf("shouldResumeUnattendedSession = %v, want %v", got, tc.want)
			}
		})
	}
}

// The user's own recipe for a restored host is to type 继续 and press enter. The host has
// to do it, or an unattended session sits still after every restore (2026-10-03).
func TestResumeUnattendedSessionQueuesOncePerHostRun(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	t.Cleanup(func() { notePreviousHostRunExit(desktopExitVerdict{}) })
	notePreviousHostRunExit(desktopExitVerdict{Kind: exitKindKilled})
	app.heartbeat = &HeartbeatEngine{unattended: true}
	tab := &WorkspaceTab{goal: "把 docs 做完"}

	if !app.resumeUnattendedSessionAfterAnInterruptedRun(tab, ctrl) {
		t.Fatal("an interrupted run with an unattended contract must queue the resume turn")
	}
	if app.resumeUnattendedSessionAfterAnInterruptedRun(tab, ctrl) {
		t.Fatal("the resume turn is queued once per host run")
	}

	app.heartbeat = &HeartbeatEngine{}
	if app.resumeUnattendedSessionAfterAnInterruptedRun(&WorkspaceTab{goal: "别的任务"}, ctrl) {
		t.Fatal("the master switch is the gate")
	}

	app.heartbeat = &HeartbeatEngine{unattended: true}
	if app.resumeUnattendedSessionAfterAnInterruptedRun(&WorkspaceTab{}, ctrl) {
		t.Fatal("a session with no Goal has no contract to carry out")
	}

	notePreviousHostRunExit(desktopExitVerdict{Kind: exitKindClean})
	if app.resumeUnattendedSessionAfterAnInterruptedRun(&WorkspaceTab{goal: "又来一个"}, ctrl) {
		t.Fatal("a clean exit interrupted nothing")
	}
}

// A recovered queue is paused, so the resume turn would wait for a click on 继续执行 that
// nobody is there to make: the unattended session has to take that gate down itself
// (the user's screenshot of the banner, 2026-10-03).
func TestResumeUnattendedSessionClearsTheGatesACrashLeft(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	t.Cleanup(func() { notePreviousHostRunExit(desktopExitVerdict{}) })
	notePreviousHostRunExit(desktopExitVerdict{Kind: exitKindKilled})
	app.heartbeat = &HeartbeatEngine{unattended: true}
	if err := ctrl.SetInboxPaused(true); err != nil {
		t.Fatalf("pause the inbox: %v", err)
	}
	if !ctrl.InboxSnapshot().Paused {
		t.Fatal("the inbox has to start paused for this to mean anything")
	}

	if !app.resumeUnattendedSessionAfterAnInterruptedRun(&WorkspaceTab{goal: "把 docs 做完"}, ctrl) {
		t.Fatal("the resume turn must be queued")
	}
	if ctrl.InboxSnapshot().Paused {
		t.Fatal("the unattended resume must leave no paused inbox behind")
	}
}

// A stop is not an erasure: the goal text is the contract a restart resumes, and the crash
// itself is what stops the goal. Filtering on "running" therefore deleted exactly the goal
// an unattended host had to pick up, and the resume could never fire again (2026-10-03).
func TestPersistedTabGoalKeepsAStoppedContract(t *testing.T) {
	tab := &WorkspaceTab{goal: "把 docs 做完"}
	if tab.Ctrl != nil {
		t.Fatal("this test is about a tab whose controller is gone")
	}
	if got := persistedTabGoal(tab); got != "把 docs 做完" {
		t.Fatalf("persistedTabGoal = %q, want the contract kept", got)
	}
}

// A crashed host restores into a tab whose own goal is empty (its status is stopped), so the
// contract has to come from the persisted tab file.
func TestUnattendedGoalContractFallsBackToThePersistedTabFile(t *testing.T) {
	_, ctrl, home := agentBusEnrolApp(t)
	// The tab file lives under the Reasonix home, which is a different resolver from the
	// state home the harness isolates: point both at the same temp directory.
	t.Setenv("REASONIX_HOME", home)
	session := ctrl.SessionPath()
	body, err := json.Marshal(desktopTabsFile{Tabs: []desktopTabEntry{
		{ID: "tab_x", SessionPath: session, Goal: "把 docs 做完"},
	}})
	if err != nil {
		t.Fatalf("marshal tabs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, tabsFileName), body, 0o644); err != nil {
		t.Fatalf("write tabs: %v", err)
	}

	if got := unattendedGoalContract(&WorkspaceTab{}, session); got != "把 docs 做完" {
		t.Fatalf("contract = %q, want the persisted goal", got)
	}
	if got := unattendedGoalContract(&WorkspaceTab{goal: "当前这份"}, session); got != "当前这份" {
		t.Fatalf("contract = %q, want the tab's own goal first", got)
	}
	if got := unattendedGoalContract(&WorkspaceTab{}, session+"-other"); got != "" {
		t.Fatalf("contract = %q, want nothing for a session the file does not name", got)
	}
}

// A bare 继续 tells the model nothing about what it was doing: the turn has to name the
// contract it was under, the instruction it was on, what the crash left queued, and that it
// must not improvise (2026-10-03).
func TestUnattendedResumeTextNamesTheContextAndForbidsImprovising(t *testing.T) {
	text := unattendedResumeText(unattendedResumeContext{
		Contract: "把 docs 做完",
		LastUser: "继续验证 T9-4",
		Pending:  []string{"等待确认的收件箱条目"},
	})
	for _, want := range []string{"上一次宿主运行被中断", "把 docs 做完", "继续验证 T9-4", "等待确认的收件箱条目", "不要重做已完成的工作"} {
		if !strings.Contains(text, want) {
			t.Fatalf("resume text misses %q: %s", want, text)
		}
	}
	if strings.TrimSpace(text) == "继续" {
		t.Fatal("the resume turn must not be a bare 继续")
	}
}

// What the hook queues is that brief, marked as the host's own so a person can tell it apart.
func TestResumeUnattendedSessionQueuesTheContextualBrief(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	t.Cleanup(func() { notePreviousHostRunExit(desktopExitVerdict{}) })
	notePreviousHostRunExit(desktopExitVerdict{Kind: exitKindKilled})
	app.heartbeat = &HeartbeatEngine{unattended: true}

	if !app.resumeUnattendedSessionAfterAnInterruptedRun(&WorkspaceTab{goal: "把 docs 做完"}, ctrl) {
		t.Fatal("the resume turn must be queued")
	}
	snapshot := ctrl.InboxSnapshot()
	for _, item := range snapshot.Items {
		if item.Source != unattendedResumeSource {
			continue
		}
		if !strings.Contains(item.Preview, "把 docs 做完") || !strings.Contains(item.Preview, "被中断") {
			t.Fatalf("queued resume preview = %q, want the contextual brief", item.Preview)
		}
		return
	}
	t.Fatalf("no %s item in %+v", unattendedResumeSource, snapshot.Items)
}

// The contract has to outlive the desktop's own tab file, which is rewritten from a stopped
// goal and therefore erases it on the next save (found on a real machine, 2026-10-03).
func TestUnattendedGoalContractSurvivesTheTabFileErasingIt(t *testing.T) {
	_, ctrl, home := agentBusEnrolApp(t)
	t.Setenv("REASONIX_HOME", home)
	session := ctrl.SessionPath()
	rememberUnattendedGoalContract(session, "把 docs 做完")

	if got := rememberedUnattendedGoalContract(session); got != "把 docs 做完" {
		t.Fatalf("remembered contract = %q, want the contract", got)
	}
	// Neither the tab nor the persisted file knows it any more: the store still does.
	if got := unattendedGoalContract(&WorkspaceTab{}, session); got != "把 docs 做完" {
		t.Fatalf("contract = %q, want the remembered one", got)
	}
	if got := rememberedUnattendedGoalContract(session + "-other"); got != "" {
		t.Fatalf("contract = %q, want nothing for another session", got)
	}
}

// Seeing a contract is what keeps it: resolution has to record it, or the first save after a
// stop is the last time the desktop ever sees the goal text.
func TestUnattendedGoalContractResolutionRecordsWhatItSees(t *testing.T) {
	_, ctrl, home := agentBusEnrolApp(t)
	t.Setenv("REASONIX_HOME", home)
	session := ctrl.SessionPath()
	body, err := json.Marshal(desktopTabsFile{Tabs: []desktopTabEntry{
		{ID: "tab_x", SessionPath: session, Goal: "把 docs 做完"},
	}})
	if err != nil {
		t.Fatalf("marshal tabs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, tabsFileName), body, 0o644); err != nil {
		t.Fatalf("write tabs: %v", err)
	}
	if got := unattendedGoalContract(&WorkspaceTab{}, session); got != "把 docs 做完" {
		t.Fatalf("contract = %q, want the persisted goal", got)
	}
	if err := os.Remove(filepath.Join(home, tabsFileName)); err != nil {
		t.Fatalf("remove tabs: %v", err)
	}
	if got := unattendedGoalContract(&WorkspaceTab{}, session); got != "把 docs 做完" {
		t.Fatalf("contract = %q, want the goal recorded while it was visible", got)
	}
}

// The host's interruption notice arrives as the last user-role message, but it is not an
// instruction: quoting it as "the instruction you were on" gives the model a wrapper.
func TestLastRealUserInstructionSkipsTheHostsRecoveryNotice(t *testing.T) {
	if !isHostRecoveryText("<interrupted-turn-recovery>\nThe previous turn was interrupted.") {
		t.Fatal("the host's recovery wrapper is not an instruction")
	}
	if !isHostRecoveryText("  [interrupted-turn-recovery] ...") {
		t.Fatal("the bracketed form is the same notice")
	}
	if isHostRecoveryText("继续验证 T9-4") {
		t.Fatal("a real instruction must not be filtered out")
	}
}
