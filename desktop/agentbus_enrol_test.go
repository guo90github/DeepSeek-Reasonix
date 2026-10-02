package main

import (
	"path/filepath"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

func agentBusEnrolApp(t *testing.T) (*App, *control.Controller, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", home)
	ctrl := control.New(control.Options{
		SessionDir:  t.TempDir(),
		SessionPath: filepath.Join(t.TempDir(), "session.jsonl"),
		Sink:        event.Discard,
	})
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: ctrl}}, activeTabID: "t1"}
	return app, ctrl, home
}

func TestAgentBusJoinEnrolsTheSessionOnTheDefaultBoard(t *testing.T) {
	app, ctrl, home := agentBusEnrolApp(t)

	before, err := app.AgentBusStatus()
	if err != nil {
		t.Fatalf("status before join: %v", err)
	}
	if before.Enrolled {
		t.Fatal("a session that never joined must not report itself enrolled")
	}
	want := filepath.Join(home, "agentbus", "default")
	if before.DefaultDir != want {
		t.Fatalf("defaultDir = %q, want %q", before.DefaultDir, want)
	}

	joined, err := app.AgentBusJoin()
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if !joined.Enrolled || joined.BoardDir != want || joined.Board != "default" {
		t.Fatalf("joined = %+v, want the default board", joined)
	}
	if ctrl.AgentBusDir() != want {
		t.Fatalf("controller board = %q, want the board the status reported", ctrl.AgentBusDir())
	}
	if joined.Participant == "" {
		t.Fatal("joining must resolve a board identity, not leave the participant empty")
	}

	left, err := app.AgentBusLeave()
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if left.Enrolled || ctrl.AgentBusDir() != "" {
		t.Fatalf("left = %+v (board %q), want no board", left, ctrl.AgentBusDir())
	}
}

// A blank session has a board but no identity on it yet. The panel has to be able to
// say that — the alternative is a join that silently shows nothing.
func TestAgentBusJoinOnAPathlessSessionPendsUntilItHasAnIdentity(t *testing.T) {
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: ctrl}}, activeTabID: "t1"}

	joined, err := app.AgentBusJoin()
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if !joined.Enrolled {
		t.Fatalf("joined = %+v, want the board to be recorded", joined)
	}
	if joined.Participant != "" || joined.Board != "" {
		t.Fatalf("joined = %+v, want a pending identity rather than a guessed one", joined)
	}
}

func TestAgentBusJoinAndLeaveNeedASession(t *testing.T) {
	if _, err := (&App{}).AgentBusJoin(); err == nil {
		t.Fatal("joining with no active session must be refused, not silently ignored")
	}
	if _, err := (&App{}).AgentBusLeave(); err == nil {
		t.Fatal("leaving with no active session must be refused")
	}
	status, err := (&App{}).AgentBusStatus()
	if err != nil {
		t.Fatalf("status with no active session: %v", err)
	}
	if status.Enrolled {
		t.Fatalf("status = %+v, want not-on-a-board", status)
	}
}
