package boot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// TestEffectAgentBusViewRidesTheTurnNotThePrefix pins guard three at the real
// provider boundary: the blackboard delta reaches the request body, carries only
// this participant's rows, and never enters the cache-stable system prefix.
func TestEffectAgentBusViewRidesTheTurnNotThePrefix(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-effect-agentbus", func(provider.Config) (provider.Provider, error) {
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
kind = "boot-effect-agentbus"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	// A session's board identity is its branch id; a controller minted without a
	// path has none, so name one here exactly as a host would.
	ctrl.SetSessionPath(filepath.Join(dir, "sessions", "board-peer.jsonl"))
	participant := agent.BranchID(ctrl.SessionPath())
	if participant == "" {
		t.Fatal("no session identity to address on a board")
	}
	busDir := filepath.Join(dir, "agentbus-board")
	sessions, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "evidence"}}
	if _, err := sessions.ApplyAll(context.Background(),
		board.Op{Verb: board.VerbAssert, Node: "mine", Actor: participant, Evidence: evidence},
		board.Op{Verb: board.VerbAssert, Node: "theirs", Actor: "someone-else", Evidence: evidence},
	); err != nil {
		t.Fatalf("apply ops: %v", err)
	}
	ctrl.SetAgentBusDir(busDir)

	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	body := conversationText(reqs[0])
	if !strings.Contains(body, "<agentbus-view>") {
		t.Fatalf("the view never reached the turn body:\n%s", body)
	}
	if !strings.Contains(body, "node id=mine") {
		t.Fatalf("my own node is missing from the request:\n%s", body)
	}
	if strings.Contains(body, "node id=theirs") {
		t.Fatalf("another participant's node reached my request:\n%s", body)
	}
	for _, message := range reqs[0].Messages {
		if message.Role == provider.RoleSystem && strings.Contains(message.Content, "agentbus") {
			t.Fatalf("the view must never enter the system prefix:\n%s", message.Content)
		}
	}
}

// TestEffectAgentBusStaysOutOfTheTurnWhenUnwired is the zero-change half of the
// same guard: a controller nobody enrolled composes exactly as before.
func TestEffectAgentBusStaysOutOfTheTurnWhenUnwired(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-effect-agentbus-off", func(provider.Config) (provider.Provider, error) {
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
kind = "boot-effect-agentbus-off"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	if body := conversationText(reqs[0]); strings.Contains(body, "agentbus") {
		t.Fatalf("an unenrolled controller must not carry a view:\n%s", body)
	}
}

func conversationText(req provider.Request) string {
	var b strings.Builder
	for _, message := range req.Messages {
		if message.Role == provider.RoleSystem {
			continue
		}
		b.WriteString(message.Content)
		b.WriteString("\n")
	}
	return b.String()
}
