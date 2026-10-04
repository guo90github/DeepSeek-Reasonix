package control

import (
	"testing"

	"reasonix/internal/agentbus"
)

// A session that answers no endpoint is still on the board. Until the desktop announced with
// no host, its participant only ever appeared in the roster when it left, so the roster could
// not answer "who is here" at all (F49, 2026-10-05).
func TestAPresenceAnnouncementWithNoEndpointRegistersTheSession(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "bob")

	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatalf("announce: %v", err)
	}

	directory, err := agentbus.OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, found, err := directory.Lookup("bob")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("the roster does not list a session that just announced itself")
	}
	if ref.Withdrawn {
		t.Fatalf("the announcement reads as a withdrawal: %+v", ref)
	}
	if ref.SessionPath != c.SessionPath() {
		t.Fatalf("sessionPath = %q, want the announcer's own %q", ref.SessionPath, c.SessionPath())
	}

	// Leaving is what retires it, and that stays visible rather than deleting the row.
	if err := c.AgentBusWithdraw(); err != nil {
		t.Fatal(err)
	}
	if ref, found, err := directory.Lookup("bob"); err != nil || found {
		t.Fatalf("a withdrawn session is still addressable: %+v (found=%t, err=%v)", ref, found, err)
	}
}
