package boot

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// The link the unattended chain rests on: a session handed board work *acts* on it. The board
// tool's own tests drive the tool directly, and the effect tests beside this file only record
// requests — neither shows the model's own tool call moving the board. This runs a real turn
// through the assembly and reads the result from another handle, as a second participant would.
func TestEffectATurnAdvancesTheBoardThroughTheBoardTool(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	kind := "boot-effect-agentbus-turn"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) {
		return testutil.NewMock("test-model",
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c1", Name: "agent_bus",
				Arguments: `{"action":"assert","node":"step","evidence":[{"kind":"test","ref":"go test ./internal/boot/"}]}`}}},
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c2", Name: "agent_bus",
				Arguments: `{"action":"claim","node":"step","steps":1}`}}},
			testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c3", Name: "agent_bus",
				Arguments: `{"action":"decide","node":"step","outcome":"done","reproducedBy":"checker","evidence":[{"kind":"test","ref":"go test ./internal/boot/"}]}`}}},
			testutil.Turn{Text: "the step is done"},
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
	ctrl.SetAgentBus(busDir, "me")

	if err := ctrl.Run(context.Background(), "take the step through the board tool"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Read the board the way another participant does: through its own handle, from the log.
	brd, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	step := state.Nodes["step"]
	if step == nil {
		t.Fatalf("the turn left no node behind: %+v", state.Nodes)
	}
	if step.Outcome != board.OutcomeDone {
		t.Fatalf("step = %+v, want the turn's own tool calls to have carried it to done", step)
	}
	if len(step.Asserts) == 0 || step.Asserts[0].Actor != "me" {
		t.Fatalf("asserts = %+v, want the turn's assertion recorded against this session", step.Asserts)
	}
}
