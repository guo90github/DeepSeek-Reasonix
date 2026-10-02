package boot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// TestBootAgentBusOptionsEnrollTheBuiltController pins the host seam: the board
// directory and participant id arriving through Options are enough for a
// pathless session to see its own view, with no session file involved.
func TestBootAgentBusOptionsEnrollTheBuiltController(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-agentbus-options", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-agentbus-options"
model = "x"
`)

	busDir := filepath.Join(dir, "board")
	sessions, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "evidence"}}
	if _, err := sessions.ApplyAll(context.Background(),
		board.Op{Verb: board.VerbAssert, Node: "mine", Actor: "peer", Evidence: evidence},
	); err != nil {
		t.Fatalf("apply ops: %v", err)
	}

	ctrl, err := Build(context.Background(), Options{
		Sink:        event.Discard,
		AgentBusDir: busDir,
		AgentBusID:  "peer",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if path := ctrl.SessionPath(); path != "" {
		t.Fatalf("this controller should be pathless, got %q", path)
	}

	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	if body := conversationText(reqs[0]); !strings.Contains(body, "node id=mine") {
		t.Fatalf("the enrolled view never reached the turn:\n%s", body)
	}
}
