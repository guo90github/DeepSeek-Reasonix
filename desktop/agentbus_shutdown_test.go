package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func agentBusParticipantLines(t *testing.T, home, participant string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, "agentbus", "default", "participants.jsonl"))
	if err != nil {
		t.Fatalf("read the board's participant log: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, participant) {
			lines = append(lines, line)
		}
	}
	return lines
}

// A host that goes away must stop being addressable: the board addresses participants by
// announcement and nothing retires it for a host that is gone — a leftover serve stayed
// addressable for a day on a real machine, where wakes sent to it were accepted and dropped
// (2026-10-03).
func TestAgentBusShutdownWithdrawsTheSessionsAddress(t *testing.T) {
	app, ctrl, home := agentBusEnrolApp(t)
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("join: %v", err)
	}
	participant := ctrl.AgentBusParticipant()
	if participant == "" {
		t.Fatal("a joined session with a session path must have an identity on the board")
	}
	if err := ctrl.AgentBusAnnounce("http://127.0.0.1:1", ""); err != nil {
		t.Fatalf("announce: %v", err)
	}
	before := agentBusParticipantLines(t, home, participant)
	if len(before) == 0 || strings.Contains(before[len(before)-1], `"withdrawn":true`) {
		t.Fatalf("announced lines = %v, want a live address before the shutdown", before)
	}

	app.withdrawAgentBusOnShutdown(ctrl)

	after := agentBusParticipantLines(t, home, participant)
	if len(after) <= len(before) || !strings.Contains(after[len(after)-1], `"withdrawn":true`) {
		t.Fatalf("lines after shutdown = %v, want the address retired", after)
	}
}
