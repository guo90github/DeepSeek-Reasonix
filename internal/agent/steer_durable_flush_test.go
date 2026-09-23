package agent

import (
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

// A durable item the turn never applied must survive turn exit: the host
// re-queues it (control.onInboxTurnDone) so the wake still runs. Consuming it
// through the loader here marked it applied at the durable layer, and turn
// completion then deleted it — a remote wake reported as delivered that never
// executed (measured 2026-09-23 against a real chatting room).
func TestFlushLeavesDurableSteerUnconsumed(t *testing.T) {
	loads := 0
	a := New(nil, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	a.steerMu.Lock()
	a.steerRunActive = true
	a.steerMu.Unlock()

	if !a.SteerItem("item-1", func() (string, error) { loads++; return "room wake", nil }) {
		t.Fatal("an active turn should accept a durable steer")
	}
	a.flushSteerQueue()

	if loads != 0 {
		t.Fatalf("flush consumed the durable item %d time(s); its fate belongs to the host", loads)
	}
	if len(a.Session().Messages) != 0 {
		t.Fatalf("a durable late steer must not write transcript rows, got %+v", a.Session().Messages)
	}
	if !a.HasUnappliedSteer() {
		t.Fatal("the host must still see that this turn left guidance unapplied")
	}
}
