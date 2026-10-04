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

// stepClaimedByBob lays down "step" and hands it to bob. A report is an assertion, and the
// board refuses an assert that carries no evidence — so the unreported case brings the node
// into being as a dependency instead, which is the one path that creates a node silently.
func stepClaimedByBob(t *testing.T, withReading bool) *Controller {
	t.Helper()
	ctx := context.Background()
	me := newAgentBusTalkController(t, t.TempDir(), "bob")
	if withReading {
		if _, err := me.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbAssert, Node: "step", Actor: "alice", Title: "hand it over",
			Evidence: []board.Evidence{{Kind: "test", Ref: "go test ./..."}},
		}); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := me.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbAssert, Node: "root", Actor: "alice", Title: "the container",
			Evidence: []board.Evidence{{Kind: "test", Ref: "go test ./..."}},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := me.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbRequire, Node: "root", Actor: "alice",
			Dep: &board.NodeSpec{ID: "step", Title: "hand it over"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := me.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "step", Actor: "bob",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	return me
}

func injectStepWake(t *testing.T, me *Controller) string {
	t.Helper()
	text, stale, ok := me.agentBusWakeInjectionRewrite(context.Background(), sessioninbox.InboxItemMeta{
		ID: "wake-1", Source: "agentbus",
		Idempotency: agentbus.DispatchKey("default", "step"),
		CreatedAt:   time.Now().UTC().Add(-time.Minute),
	})
	if !ok || stale {
		t.Fatalf("an assignment still held by bob must inject a live wake (ok=%t stale=%t):\n%s", ok, stale, text)
	}
	return text
}

// The host hands a step over with "do it, then decide it". For a step that already carries
// readings that instruction sends its holder back to redo work somebody reported on — which
// is what the real board showed, a probe reaching 21 assertions with nothing proved
// (2026-10-05).
func TestInjectedDispatchForAStepThatAlreadyCarriesReadingsAsksForTheVerdict(t *testing.T) {
	text := injectStepWake(t, stepClaimedByBob(t, true))

	if strings.Contains(text, "do it, then decide it") {
		t.Fatalf("the injected wake still tells the holder to redo reported work:\n%s", text)
	}
	for _, want := range []string{"step", "1 assertion", "decide", "action=view"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the injected wake does not carry %q:\n%s", want, text)
		}
	}
}

// A step nobody has reported on keeps the instruction the host's assignment always carried.
func TestInjectedDispatchWithoutReadingsKeepsTheDoItInstruction(t *testing.T) {
	text := injectStepWake(t, stepClaimedByBob(t, false))

	if !strings.Contains(text, "do it, then decide it") {
		t.Fatalf("an unreported step lost its instruction:\n%s", text)
	}
}
