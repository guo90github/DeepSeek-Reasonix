package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// dispatchTargetFor seeds one board, hands its only startable step out, and returns what the
// dispatcher told the claimant (with the board dir, so a test can read what was recorded).
func dispatchTargetFor(t *testing.T, reported bool) (agentbus.WakeTarget, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if reported {
		if _, err := c.ApplyAgentBusOp(ctx, busAssert("step", "alice")); err != nil {
			t.Fatalf("assert step: %v", err)
		}
	}
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require step: %v", err)
	}

	var got agentbus.WakeTarget
	deliver := func(_ context.Context, target agentbus.WakeTarget) error {
		got = target
		return nil
	}
	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("dispatch = %d (%v), want the step handed out", n, err)
	}
	return got, dir
}

// lastOpID reads the folded board and returns what wrote a node last: the only audit face that
// can tell a verification from a fresh run.
func lastOpID(t *testing.T, dir, node string) string {
	t.Helper()
	b, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	st, err := b.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	found := st.Nodes[node]
	if found == nil {
		t.Fatalf("no node %q in the folded state", node)
	}
	return found.LastOpID
}

// A step somebody already reported on is not handed over as a fresh run. Three faces have to say
// so: the wake's own key (the ledger collapses repeats by key), the line a person reads, and the
// op the board records (F15/F46, 2026-10-05).
func TestDispatchToAStepThatAlreadyCarriesReadingsAsksForAVerification(t *testing.T) {
	target, dir := dispatchTargetFor(t, true)

	if !agentbus.IsDispatchKey(target.Key) || !strings.HasSuffix(target.Key, "/verify") {
		t.Fatalf("wake key = %q, want a dispatch key marked as a verification", target.Key)
	}
	if line := AgentBusWakeLine(target); !strings.Contains(line, "复核") {
		t.Fatalf("wake line = %q, want it to say this is a verification", line)
	}
	if target.Reports != 1 {
		t.Fatalf("Reports = %d, want the reading it found", target.Reports)
	}
	if id := lastOpID(t, dir, "step"); !strings.Contains(id, "/verify/") {
		t.Fatalf("last op id = %q, want the verification recorded, not a plain hand-over", id)
	}
}

// A step nobody reported on is still handed over as work: the intent is derived from the node, not
// from the fact that a host dispatched it.
func TestDispatchToAFreshStepStaysAWorkWake(t *testing.T) {
	target, dir := dispatchTargetFor(t, false)

	if strings.HasSuffix(target.Key, "/verify") {
		t.Fatalf("wake key = %q, want the plain assignment key", target.Key)
	}
	if line := AgentBusWakeLine(target); strings.Contains(line, "复核") {
		t.Fatalf("wake line = %q, want work, not a verification", line)
	}
	if target.Reports != 0 {
		t.Fatalf("Reports = %d, want zero: nobody reported on this step", target.Reports)
	}
	if id := lastOpID(t, dir, "step"); strings.Contains(id, "/verify/") {
		t.Fatalf("last op id = %q, want a plain hand-over", id)
	}
}
