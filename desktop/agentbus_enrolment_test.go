package main

import (
	"path/filepath"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

// A board membership has to outlive the process. Without this, the OS watchdog restores
// the app and the board stops advancing at exactly the moment it came back: the session
// that was working on it is no longer on the board (found on a real machine, 2026-10-03).
func TestAgentBusEnrolmentSurvivesARestart(t *testing.T) {
	app, ctrl, home := agentBusEnrolApp(t)
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("join: %v", err)
	}
	want := filepath.Join(home, "agentbus", "default")
	record, ok := rememberedAgentBusEnrolment(ctrl.SessionPath())
	if !ok || record.Dir != want {
		t.Fatalf("remembered = (%+v, %v), want the board this session joined", record, ok)
	}

	// The relaunched host builds a fresh controller for the same session: it starts off
	// the board, and the boot path puts it back.
	restarted := control.New(control.Options{
		SessionDir:  t.TempDir(),
		SessionPath: ctrl.SessionPath(),
		Sink:        event.Discard,
	})
	if restarted.AgentBusEnrolled() {
		t.Fatal("a freshly built controller starts off the board")
	}
	app.restoreAgentBusEnrolment(restarted)
	if !restarted.AgentBusEnrolled() {
		t.Fatal("a session that joined before must be put back on its board")
	}
	if restarted.AgentBusDir() != want {
		t.Fatalf("board = %q, want %q", restarted.AgentBusDir(), want)
	}

	// Leaving is a decision, so it must not be re-applied on the next start.
	if _, err := app.AgentBusLeave(); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, ok := rememberedAgentBusEnrolment(ctrl.SessionPath()); ok {
		t.Fatal("leaving must forget the board")
	}
	next := control.New(control.Options{
		SessionDir:  t.TempDir(),
		SessionPath: ctrl.SessionPath(),
		Sink:        event.Discard,
	})
	app.restoreAgentBusEnrolment(next)
	if next.AgentBusEnrolled() {
		t.Fatal("a session that left must not be put back on a board")
	}
}

// A session with no path cannot be recognised again, so there is nothing to remember.
func TestAgentBusEnrolmentNeedsASessionPath(t *testing.T) {
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	if err := rememberAgentBusEnrolment("", "/board", "alice"); err != nil {
		t.Fatalf("a pathless session must be a no-op, not an error: %v", err)
	}
	if _, ok := rememberedAgentBusEnrolment(""); ok {
		t.Fatal("nothing is remembered for a session with no path")
	}
}

// The path must come from the tab, not from the controller. A controller learns its
// session only once the session is bound, so the build-time attempt reads "" and restores
// nothing — the first cut of this fix, which is why the board still stopped advancing on a
// real machine after the watchdog restored the app (2026-10-03).
func TestRestoreAgentBusEnrolmentUsesTheTabPathNotTheControllerPath(t *testing.T) {
	app, _, home := agentBusEnrolApp(t)
	session := filepath.Join(home, "sessions", "bound-later.jsonl")
	board := filepath.Join(home, "agentbus", "default")
	if err := rememberAgentBusEnrolment(session, board, ""); err != nil {
		t.Fatalf("remember: %v", err)
	}

	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	app.restoreAgentBusEnrolment(ctrl)
	if ctrl.AgentBusEnrolled() {
		t.Fatal("a controller that does not know its session yet must not join a board")
	}

	app.restoreAgentBusEnrolmentFor(session, ctrl)
	if !ctrl.AgentBusEnrolled() {
		t.Fatal("the path its tab knew must put the session back on its board")
	}
	// The board itself is only reported by a session that has an identity on it, which a
	// controller here has not been given; what this test is about is the enrolment.
	app.restoreAgentBusEnrolmentFor(session, ctrl)
	if !ctrl.AgentBusEnrolled() {
		t.Fatal("a second restore must leave the session on its board")
	}
}
