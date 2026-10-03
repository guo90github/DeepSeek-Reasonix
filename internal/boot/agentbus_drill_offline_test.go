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
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func wakeTargetsFor(t *testing.T, busDir string) []agentbus.WakeTarget {
	t.Helper()
	brd, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return agentbus.WakeTargets(agentbus.WakeInput{State: state, Now: time.Now().UTC()})
}

func wakeTargetFor(targets []agentbus.WakeTarget, participant string) (agentbus.WakeTarget, bool) {
	for _, target := range targets {
		if target.Participant == participant {
			return target, true
		}
	}
	return agentbus.WakeTarget{}, false
}

// ORCHESTRATION.md §5.1.1's single-machine approximation, which the recipe says was never run:
// a block is addressed to one session, that session takes it with its own tool calls, and the
// kernel's own derivation then moves the deliverable's groups outward. The recipe's whole table
// lives on the shared board, so one board plus one really assembled session covers its board-side
// assertions (steps 1–5); only "A delivers the wake into B's process" stays real-machine.
func TestEffectASessionHandedAnAssignedBlockAdvancesTheDeliverable(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	kind := "boot-effect-agentbus-drill"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) {
		return testutil.NewMock("test-model",
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c1", Name: "agent_bus",
				Arguments: `{"action":"view"}`}}},
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c2", Name: "agent_bus",
				Arguments: `{"action":"claim","node":"block","steps":1}`}}},
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c3", Name: "agent_bus",
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

	ctx := context.Background()
	brd, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	// Steps 1–2 as the orchestrator participant would write them: one node waiting on the
	// deliverable (so the deliverable has a requester to wake), the deliverable waiting on the
	// block, and the block addressed to this session alone.
	for _, op := range []board.Op{
		{Verb: board.VerbAssert, Node: "final", Actor: "me", Title: "ship it",
			Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:final"}}},
		{Verb: board.VerbAssert, Node: "deliverable", Actor: "me", Title: "the deliverable",
			Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:deliverable"}}},
		{Verb: board.VerbRequire, Node: "final", Actor: "me", Dep: &board.NodeSpec{ID: "deliverable"}},
		{Verb: board.VerbAssert, Node: "block", Actor: "me",
			Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:block"}}},
		{Verb: board.VerbRequire, Node: "deliverable", Actor: "me", Dep: &board.NodeSpec{ID: "block"}},
		{Verb: board.VerbAssign, Node: "block", Actor: "me", Assignee: "worker"},
	} {
		if _, err := brd.Apply(ctx, op); err != nil {
			t.Fatalf("apply %s on %s: %v", op.Verb, op.Node, err)
		}
	}

	// The wake side of the hand-off: the addressed session is owed the block by name, and the
	// node that waits on the deliverable is owed nothing until the block lands.
	targets := wakeTargetsFor(t, busDir)
	worker, ok := wakeTargetFor(targets, "worker")
	if !ok || !slices.Contains(worker.Assigned, "block") {
		t.Fatalf("targets = %+v, want the addressed session owed the block by name", targets)
	}
	if owner, ok := wakeTargetFor(targets, "me"); ok && slices.Contains(owner.Ready, "deliverable") {
		t.Fatalf("the deliverable was offered before its block was done: %+v", owner)
	}

	// Steps 3–4: the session acts on what the board addressed to it.
	if err := ctrl.Run(ctx, "the board addressed this step to you: take it and finish it"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ops, err := brd.Ops(ctx)
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	block := state.Nodes["block"]
	if block == nil || block.Outcome != board.OutcomeDone {
		t.Fatalf("block = %+v, want the addressed session to have carried it to done", block)
	}
	for _, verb := range []board.Verb{board.VerbClaim, board.VerbDecide} {
		if !slices.ContainsFunc(ops, func(op board.Op) bool {
			return op.Node == "block" && op.Verb == verb && op.Actor == "worker"
		}) {
			t.Fatalf("no %s on block by the addressed session: %+v", verb, ops)
		}
	}

	// Step 5: the deliverable's group moves outward — startable, unowned and still waited on, so
	// it is now offered to whoever asked for it, and the finished block is no longer offered.
	targets = wakeTargetsFor(t, busDir)
	owner, ok := wakeTargetFor(targets, "me")
	if !ok || !slices.Contains(owner.Ready, "deliverable") {
		t.Fatalf("targets = %+v, want the deliverable offered to its requester now", targets)
	}
	if slices.Contains(owner.Ready, "block") {
		t.Fatalf("the done block is still offered: %+v", owner.Ready)
	}
}
