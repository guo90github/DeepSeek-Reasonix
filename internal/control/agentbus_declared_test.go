package control

import (
	"testing"

	"reasonix/internal/agentbus"
)

// An announcement carries what the session is, not only where it is: the roster is where a
// reader decides who to hand work to, and an opaque id answers nothing (F48, 2026-10-05).
func TestAnAnnouncementCarriesTheSessionsOwnDeclaredFacts(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "bob")
	c.workspaceRoot = "/work/alpha"
	c.selection.ref = "deepseek-flash"

	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatal(err)
	}

	directory, err := agentbus.OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, found, err := directory.Lookup("bob")
	if err != nil || !found {
		t.Fatalf("lookup = (%+v, %t, %v), want the announcement", ref, found, err)
	}
	if ref.Workspace != "/work/alpha" || ref.Model != "deepseek-flash" {
		t.Fatalf("declared facts = workspace %q model %q, want what the session reports", ref.Workspace, ref.Model)
	}
}
