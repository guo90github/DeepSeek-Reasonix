package agentbus

import (
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// The lease length is what tells one way of claiming from another — a hand-written claim
// defaults to 900s and the host's dispatch to 1800s — and the row used to carry only the
// deadline, so a reader could not tell which one it was (F52, 2026-10-05).
func TestAViewRowNamesTheLeaseWhileItIsHeld(t *testing.T) {
	claimed := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	st := board.NewState()
	st.Nodes["held"] = &board.Node{
		ID: "held", Title: "under lease", State: board.StateClaimed, Owner: "bob",
		ClaimedAt: claimed, Deadline: claimed.Add(15 * time.Minute), LastSeq: 2,
	}
	st.Nodes["pool"] = &board.Node{
		ID: "pool", Title: "nobody holds it", State: board.StateOpen,
		Requesters: []string{"bob"}, LastSeq: 3,
	}

	rendered := BuildView(st, ViewSpec{Board: "default", Participant: "bob"}).Render()
	if !strings.Contains(rendered, "lease=15m0s") {
		t.Fatalf("the held row does not name its lease:\n%s", rendered)
	}
	if got := strings.Count(rendered, "lease="); got != 1 {
		t.Fatalf("%d rows carry a lease, want only the held one:\n%s", got, rendered)
	}
	// A row nobody holds ends at its title, exactly as it did before lease existed.
	if !strings.Contains(rendered, "title=\"nobody holds it\"\n") {
		t.Fatalf("the unheld row is not byte-stable:\n%s", rendered)
	}
}
