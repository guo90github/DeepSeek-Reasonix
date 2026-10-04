package control

import (
	"context"
	"errors"
	"testing"

	"reasonix/internal/agentbus"
)

// A board outlives the sessions that wrote to it, so a wake target can name a participant that is
// simply not here. That is not a delivery that failed — nobody was ever going to receive it — and
// it must neither be counted as a brake nor be re-attempted on every tick. The contrast in one
// test: one participant is gone for good, the other is here but unreachable right now.
// (2026-10-05, real machine: a board carrying four departed participants was re-attempted every 30
// seconds per tab, each attempt adding a row to the host's wake-failure alarm.)
func TestAWakeWithNoRouteIsNotADeliveryFailureAndIsNotRetried(t *testing.T) {
	ctx := context.Background()
	ctrl := newAgentBusTalkController(t, t.TempDir(), "orchestrator")
	gone := "gone-" + t.Name()
	unreachable := "unreachable-" + t.Name()
	noRouteAttempts, failedAttempts := 0, 0
	ctrl.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		if target.Participant == gone {
			noRouteAttempts++
			return agentbus.NoRoute(target.Participant)
		}
		failedAttempts++
		return errors.New("connection refused")
	})
	failuresBefore := AgentBusWakeFailures().Count

	if _, err := ctrl.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", gone, "schema-one")); err != nil {
		t.Fatalf("require for the departed participant: %v", err)
	}
	if _, err := ctrl.ApplyAgentBusOp(ctx, busRequire("design", unreachable, "schema-two")); err != nil {
		t.Fatalf("require for the unreachable participant: %v", err)
	}
	if noRouteAttempts != 1 || failedAttempts == 0 {
		t.Fatalf("attempts = (no route %d, failed %d), want each target tried", noRouteAttempts, failedAttempts)
	}
	if got := AgentBusWakeFailures().Count; got != failuresBefore+1 {
		t.Fatalf("failures = %d, want exactly the one delivery that failed (%d)", got, failuresBefore+1)
	}

	// Ticking changes nothing for the participant that is not here, and retries the one that is.
	for range 3 {
		if n := ctrl.WakeAgentBus(ctx); n != 0 {
			t.Fatalf("a tick reported %d wakes, want 0", n)
		}
	}
	if noRouteAttempts != 1 {
		t.Fatalf("the departed participant was re-attempted %d times, want once per work set", noRouteAttempts)
	}
	if failedAttempts < 3 {
		t.Fatalf("a real delivery failure was not retried: %d attempts", failedAttempts)
	}

	seen := 0
	for _, participant := range AgentBusWakeUnreachable() {
		if participant == gone {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("unreachable participants = %v, want %q listed once", AgentBusWakeUnreachable(), gone)
	}
}
