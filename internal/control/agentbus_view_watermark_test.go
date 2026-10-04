package control

import (
	"context"
	"strings"
	"testing"
)

// The view rides the same watermark as talk, and F26's fix carries both because they travel
// as one value: re-enrolling must not re-deliver rows a turn already carried (2026-10-05).
func TestReEnrollingKeepsTheViewWatermark(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	me := newAgentBusTalkController(t, dir, "bob")

	if _, err := me.ApplyAgentBusOp(ctx, busAssert("design", "bob")); err != nil {
		t.Fatal(err)
	}

	first := me.agentBusTurnBlock()
	if !strings.Contains(first, "design") {
		t.Fatalf("the first turn block does not carry bob's node:\n%s", first)
	}

	me.SetAgentBus(dir, "bob")
	if second := me.agentBusTurnBlock(); second != "" {
		t.Fatalf("re-enrolling re-delivered a row the turn already carried:\n%s", second)
	}
}
