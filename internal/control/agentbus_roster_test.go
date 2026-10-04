package control

import "testing"

// The roster answers "who is on this board": a session that announced itself is listed, and one
// that withdrew is gone from the list rather than left as a row nobody can use
// (F48/F49, 2026-10-05).
func TestTheRosterListsWhoAnnouncedAndDropsWhoWithdrew(t *testing.T) {
	c := newAgentBusTalkController(t, t.TempDir(), "bob")
	if err := c.AgentBusAnnounce("", ""); err != nil {
		t.Fatal(err)
	}

	refs, err := c.AgentBusParticipants()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Participant != "bob" {
		t.Fatalf("roster = %+v, want bob alone", refs)
	}

	if err := c.AgentBusWithdraw(); err != nil {
		t.Fatal(err)
	}
	if refs, err := c.AgentBusParticipants(); err != nil || len(refs) != 0 {
		t.Fatalf("roster after withdrawing = %+v (err=%v), want empty", refs, err)
	}
}
