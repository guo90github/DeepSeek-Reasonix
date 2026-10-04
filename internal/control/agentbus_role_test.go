package control

import (
	"testing"

	"reasonix/internal/agentbus"
)

// The roster renders a role column, and the only honest source for it is the operator's own
// word: the kernel has no vocabulary of its own. A host declares it, the announcement carries
// it, and a re-enrolment keeps it (F48, 2026-10-05).
func TestAnAnnouncementCarriesTheRoleTheHostDeclared(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "bob")
	c.SetAgentBusRole("reviewer")

	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatal(err)
	}
	if ref := announcedRef(t, dir); ref.Role != "reviewer" {
		t.Fatalf("role = %q, want the declared one", ref.Role)
	}

	// Re-enrolling the same board keeps the host's word: it describes this session, not the
	// board it happens to be on.
	c.SetAgentBus(dir, "bob")
	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatal(err)
	}
	if ref := announcedRef(t, dir); ref.Role != "reviewer" {
		t.Fatalf("role after re-enrolment = %q, want it kept", ref.Role)
	}
}

// An undeclared role stays undeclared: the board never invents one for a session.
func TestAnUndeclaredRoleStaysEmpty(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "bob")

	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatal(err)
	}
	if ref := announcedRef(t, dir); ref.Role != "" {
		t.Fatalf("role = %q, want it empty", ref.Role)
	}
}

func announcedRef(t *testing.T, dir string) agentbus.ParticipantRef {
	t.Helper()
	directory, err := agentbus.OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, found, err := directory.Lookup("bob")
	if err != nil || !found {
		t.Fatalf("lookup = (%+v, %t, %v), want the announcement", ref, found, err)
	}
	return ref
}
