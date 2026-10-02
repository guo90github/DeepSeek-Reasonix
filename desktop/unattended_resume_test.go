package main

import (
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
