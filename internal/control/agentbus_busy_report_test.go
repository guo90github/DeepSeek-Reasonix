package control

import "testing"

// "Busy with what" and "for how long" are the two facts a peer needs before deciding whether to
// wait or take the step itself. An idle session says nothing at all, so a roster row cannot make
// an idle peer look busy (F57, 2026-10-05).
func TestTheBusyReportNamesTheTurnAndStaysSilentWhenIdle(t *testing.T) {
	c := newAgentBusTalkController(t, t.TempDir(), "bob")
	if got := c.busyReport(); got != "" {
		t.Fatalf("an idle session reports %q, want nothing", got)
	}

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	if got := c.busyReport(); got != "running a turn" {
		t.Fatalf("a session in a turn reports %q, want the turn it is in", got)
	}
}
