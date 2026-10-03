package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/boot"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

func agentBusAnnounceLines(t *testing.T, dir string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "participants.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read the board's participant log: %v", err)
	}
	return strings.Count(string(raw), "\n")
}

// A serve host has no turn loop, so without a tick nothing advances its board: a lapsed lease
// stays held and the address is never renewed — a wake is addressed by announcement, and one
// that is never republished goes stale (AGENT_BUS §11.5, 2026-10-03).
//
// The host below does carry an endpoint: a host without one publishes nothing at all (the same
// guard the boot-time announcement uses), so a test that omitted it would assert the opposite.
func TestAgentBusTickRepublishesTheAddress(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", home)
	dir := filepath.Join(home, "agentbus", "default")
	ctrl := control.New(control.Options{
		SessionDir:  t.TempDir(),
		SessionPath: filepath.Join(t.TempDir(), "session.jsonl"),
		Sink:        event.Discard,
	})
	defer ctrl.Close()
	ctrl.SetAgentBus(dir, "")
	if err := ctrl.AgentBusAnnounce("http://127.0.0.1:1", ""); err != nil {
		t.Fatalf("announce: %v", err)
	}
	before := agentBusAnnounceLines(t, dir)
	if before == 0 {
		t.Fatal("the first announcement wrote nothing to the board")
	}

	stop := startAgentBusTick(ctrl, boot.Options{AgentBusDir: dir, AgentBusHost: "http://127.0.0.1:1"}, 20*time.Millisecond)
	defer stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if agentBusAnnounceLines(t, dir) > before {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the headless tick never republished the address (still %d lines)", before)
}

// A host that is not on a board must not start a ticker at all: the same switch that enrols a
// session decides whether it runs.
func TestAgentBusTickWithoutABoardDoesNothing(t *testing.T) {
	stop := startAgentBusTick(nil, boot.Options{}, time.Millisecond)
	stop()
	stop()
}
