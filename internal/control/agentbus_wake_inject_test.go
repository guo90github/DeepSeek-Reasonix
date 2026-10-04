package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/sessioninbox"
)

// A wake is rendered when it is sent and injected when the turn ends, so what the model
// reads has to be the board as it is at injection — never the list the sender froze.
func TestInjectedWakeIsRebuiltAgainstTheBoardAsItIsNow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	me := newAgentBusTalkController(t, dir, "bob")
	if _, err := me.ApplyAgentBusOp(ctx, busAssert("design", "bob")); err != nil {
		t.Fatal(err)
	}
	if _, err := me.ApplyAgentBusOp(ctx, busRequire("design", "bob", "step")); err != nil {
		t.Fatal(err)
	}
	_, input, err := me.agentBusWakeSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var queued agentbus.WakeTarget
	for _, target := range agentbus.WakeTargets(input) {
		if target.Participant == "bob" {
			queued = target
		}
	}
	if queued.Key == "" {
		t.Fatalf("the board produced no wake for bob: %+v", agentbus.WakeTargets(input))
	}
	if frozen := AgentBusWakePrompt(queued); !strings.Contains(frozen, "step") {
		t.Fatalf("the sent wake does not name the work: %q", frozen)
	}
	meta := sessioninbox.InboxItemMeta{
		ID: "wake-1", Source: "agentbus", Idempotency: queued.Key,
		CreatedAt: time.Now().UTC().Add(-5 * time.Minute),
	}

	// The board moves on while the wake waits: the step is taken.
	if _, err := me.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "step", Actor: "bob",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	rebuilt, _, ok := me.agentBusWakeInjectionRewrite(ctx, meta)
	if !ok {
		t.Fatal("an agentbus wake must be rewritten at injection")
	}
	if strings.Contains(rebuilt, "startable now: step") {
		t.Fatalf("the injected wake still lists work that is already taken:\n%s", rebuilt)
	}
	if !strings.Contains(rebuilt, "queued 5m") {
		t.Fatalf("the injected wake does not say how long it waited:\n%s", rebuilt)
	}
}

// An assignment whose node is no longer the recipient's is refused, not re-issued: the
// wake would otherwise send the reader back to work somebody else now owns.
func TestInjectedAssignmentForWorkThatIsNoLongerYoursIsRefused(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	me := newAgentBusTalkController(t, dir, "bob")
	if _, err := me.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatal(err)
	}
	if _, err := me.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatal(err)
	}
	if _, err := me.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "step", Actor: "alice",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	meta := sessioninbox.InboxItemMeta{
		ID: "wake-2", Source: "agentbus", Idempotency: agentbus.DispatchKey("default", "step"),
		CreatedAt: time.Now().UTC().Add(-90 * time.Second),
	}

	rebuilt, stale, ok := me.agentBusWakeInjectionRewrite(ctx, meta)
	if !ok {
		t.Fatal("an assignment is rewritten at injection too")
	}
	if !stale {
		t.Fatalf("injected assignment = %q, want it reported stale so no turn is spent on it", rebuilt)
	}
	if !strings.Contains(rebuilt, "held by alice") {
		t.Fatalf("injected assignment = %q, want it refused with who holds the node now", rebuilt)
	}
	if strings.Contains(rebuilt, "claimed in your name") {
		t.Fatalf("the injected assignment still claims the node is the recipient's:\n%s", rebuilt)
	}
}

// Anything that is not a wake is injected exactly as it was queued: the rewrite is not a
// second place that can edit what a person typed.
func TestInjectedNonWakeItemIsLeftAlone(t *testing.T) {
	me := newAgentBusTalkController(t, t.TempDir(), "bob")
	if _, _, ok := me.agentBusWakeInjectionRewrite(context.Background(), sessioninbox.InboxItemMeta{
		ID: "typed", Intent: sessioninbox.IntentFollowup, Idempotency: "typed-1",
	}); ok {
		t.Fatal("a queued item that is not a wake must not be rewritten")
	}
}

// A wake whose whole list is gone is reported stale, which is what lets the caller consume the
// item without spending a turn on a block that only says so (2026-10-05: one transcript carried
// three identical "no longer holds" blocks, each as its own turn).
func TestAWakeWithNothingWaitingIsReportedStale(t *testing.T) {
	me := newAgentBusTalkController(t, t.TempDir(), "bob")
	text, stale, ok := me.agentBusWakeInjectionRewrite(context.Background(), sessioninbox.InboxItemMeta{
		ID: "wake-3", Source: "agentbus", Idempotency: "agentbus-wake:default/bob/nothing",
		CreatedAt: time.Now().UTC().Add(-2 * time.Minute),
	})
	if !ok {
		t.Fatal("a generic wake is rewritten at injection")
	}
	if !stale {
		t.Fatalf("wake = %q, want it reported stale when nothing waits on bob", text)
	}
	if !strings.Contains(text, "nothing is waiting on you") {
		t.Fatalf("wake = %q, want the refusal that names who is idle", text)
	}
}
