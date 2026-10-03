package boot

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// The chain the unattended story rests on, composed at the assembly boundary: work the board
// addressed to a session is *delivered* to that session the way a host delivers it (an inbox
// follow-up keyed by the wake, which is what starts a turn on an idle session), and the woken
// session then acts on it with its own tool calls. The dispatch alone only claims the step, so a
// `decide` op is unambiguously the session's own hand — that is the op this pins.
func TestEffectAWakeLandsInTheSessionThatThenActsOnIt(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	kind := "boot-effect-agentbus-woken"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) {
		return testutil.NewMock("test-model",
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c1", Name: "agent_bus",
				Arguments: `{"action":"view"}`}}},
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c2", Name: "agent_bus",
				Arguments: `{"action":"decide","node":"block","outcome":"done","reproducedBy":"checker","evidence":[{"kind":"test","ref":"go test ./internal/boot/"}]}`}}},
			testutil.Turn{Text: "the block is done"},
		), nil
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	busDir := filepath.Join(dir, "agentbus-board")
	ctrl.SetAgentBus(busDir, "worker")
	// A hosted session always has a persisted path — a wake has to be durable — so the fixture
	// gives this one what serve and the desktop set when they bind a session. It has no lease
	// holder, so the transcript writes below warn without failing; the wake is what this pins.
	writeFile(t, dir, "session.jsonl", "")
	ctrl.SetSessionPath(filepath.Join(dir, "session.jsonl"))

	ctx := context.Background()
	brd, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	// The orchestrator's half: a deliverable waiting on one block, and that block addressed to
	// this session by name — the shape whose assignee the kernel owes a wake.
	for _, op := range []board.Op{
		{Verb: board.VerbAssert, Node: "deliverable", Actor: "me", Title: "the deliverable",
			Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:deliverable"}}},
		{Verb: board.VerbAssert, Node: "block", Actor: "me",
			Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:block"}}},
		{Verb: board.VerbRequire, Node: "deliverable", Actor: "me", Dep: &board.NodeSpec{ID: "block"}},
		{Verb: board.VerbAssign, Node: "block", Actor: "me", Assignee: "worker"},
	} {
		if _, err := brd.Apply(ctx, op); err != nil {
			t.Fatalf("apply %s on %s: %v", op.Verb, op.Node, err)
		}
	}

	// The host's delivery, exactly as the desktop's and serve's wakers do it.
	delivered := 0
	handed, err := ctrl.AgentBusDispatch(ctx, "worker", func(_ context.Context, target agentbus.WakeTarget) error {
		text := control.AgentBusWakePrompt(target)
		delivered++
		_, err := ctrl.TryEnqueueFollowup(control.InboxRequest{
			Submit:      text,
			Raw:         text,
			Display:     control.AgentBusWakeLine(target),
			Source:      control.AgentBusWakeSource,
			Idempotency: target.Key,
		})
		return err
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if handed != 1 || delivered != 1 {
		t.Fatalf("dispatch handed out %d step(s) and delivered %d wake(s), want one each", handed, delivered)
	}

	// The wake is what starts the session's turn; it acts on its own from there.
	deadline := time.Now().Add(15 * time.Second)
	for {
		ops, err := brd.Ops(ctx)
		if err != nil {
			t.Fatalf("ops: %v", err)
		}
		if slices.ContainsFunc(ops, func(op board.Op) bool {
			return op.Node == "block" && op.Verb == board.VerbDecide && op.Actor == "worker"
		}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the woken session never decided the block: %+v", ops)
		}
		time.Sleep(20 * time.Millisecond)
	}

	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if block := state.Nodes["block"]; block == nil || block.Outcome != board.OutcomeDone {
		t.Fatalf("block = %+v, want the woken session to have carried it to done", block)
	}
}
