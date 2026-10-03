package main

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

// A machine that already uses a board keeps its sessions on it: a fresh session silently
// left off reads nothing and is told nothing, and that cost a real session a manual join
// before the work on the board became visible (2026-10-03). Leaving is the host-level
// switch that stops the fallback putting a session back.
func TestAgentBusPutsANewSessionOnTheBoardTheMachineAlreadyUses(t *testing.T) {
	app, ctrl, home := agentBusEnrolApp(t)
	boardDir := filepath.Join(home, "agentbus", "default")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("make board dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "board.jsonl"), nil, 0o644); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	app.restoreAgentBusEnrolmentFor(ctrl.SessionPath(), ctrl)
	if got := ctrl.AgentBusDir(); got != boardDir {
		t.Fatalf("board = %q, want the board this machine already uses %q", got, boardDir)
	}

	// Leaving is remembered for the host, so the next fresh session stays off the board.
	if _, err := app.AgentBusLeave(); err != nil {
		t.Fatalf("leave: %v", err)
	}
	next := control.New(control.Options{
		SessionDir:  t.TempDir(),
		SessionPath: filepath.Join(t.TempDir(), "session.jsonl"),
		Sink:        event.Discard,
	})
	app.restoreAgentBusEnrolmentFor(next.SessionPath(), next)
	if got := next.AgentBusDir(); got != "" {
		t.Fatalf("a session created after leave joined %q, want it left off", got)
	}

	// Joining again is the single switch that lifts the opt-out.
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	again := control.New(control.Options{
		SessionDir:  t.TempDir(),
		SessionPath: filepath.Join(t.TempDir(), "session.jsonl"),
		Sink:        event.Discard,
	})
	app.restoreAgentBusEnrolmentFor(again.SessionPath(), again)
	if got := again.AgentBusDir(); got != boardDir {
		t.Fatalf("a session created after a rejoin got %q, want the board back", got)
	}
}

// A host that never joined anything stays out of the way: the fallback keys on a board that
// exists on disk, not on the wish to collaborate.
func TestAgentBusLeavesAMachineWithNoBoardAlone(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	app.restoreAgentBusEnrolmentFor(ctrl.SessionPath(), ctrl)
	if got := ctrl.AgentBusDir(); got != "" {
		t.Fatalf("board = %q, want nothing while no board exists on disk", got)
	}
}
