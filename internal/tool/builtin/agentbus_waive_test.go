package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// A by-product has no artifact to point at, so a waive that demanded one would be no exit at all:
// the reason is what the board records instead (F86, 2026-10-05).
func TestAgentBusToolWaivesAByproductWithOnlyAReason(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}
	tool := NewAgentBusTool(port)

	if _, err := tool.Execute(context.Background(), boardArgs(t, `{"action":"waive","node":"probe","reason":"a probe: nothing to deliver"}`)); err != nil {
		t.Fatalf("waive: %v", err)
	}
	if len(port.applied) != 1 {
		t.Fatalf("applied %d ops, want one waive", len(port.applied))
	}
	op := port.applied[0]
	if op.Verb != board.VerbWaive || op.Reason != "a probe: nothing to deliver" {
		t.Fatalf("op = %+v, want a waive carrying the call's reason", op)
	}
	if len(op.Evidence) != 0 {
		t.Fatalf("op evidence = %+v, want none: nothing re-checkable rides with a waive", op.Evidence)
	}

	// The reason is the whole record, so an empty one never reaches the board.
	_, err := tool.Execute(context.Background(), boardArgs(t, `{"action":"waive","node":"probe"}`))
	if err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("waive without a reason = %v, want a refusal naming the reason", err)
	}
	if len(port.applied) != 1 {
		t.Fatalf("a refused waive reached the board: %+v", port.applied)
	}
}
