package boot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// TestEffectAgentBusKeepsTheCachedPrefixStableAcrossTurns is T4-7: the view is an
// append-only turn tail, so two turns with a changing board send a byte-identical
// system prefix and tool surface, and the second turn carries only the delta.
func TestEffectAgentBusKeepsTheCachedPrefixStableAcrossTurns(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register("boot-agentbus-prefix", func(provider.Config) (provider.Provider, error) {
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
kind = "boot-agentbus-prefix"
model = "x"
`)

	busDir := filepath.Join(dir, "board")
	sessions, err := board.Open(busDir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "evidence"}}
	if _, err := sessions.Apply(context.Background(),
		board.Op{Verb: board.VerbAssert, Node: "first", Actor: "peer", Evidence: evidence}); err != nil {
		t.Fatalf("seed: %v", err)
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

	bg := context.Background()
	if err := ctrl.Run(bg, "one"); err != nil {
		t.Fatalf("turn one: %v", err)
	}
	if _, err := sessions.Apply(bg,
		board.Op{Verb: board.VerbAssert, Node: "second", Actor: "peer", Evidence: evidence}); err != nil {
		t.Fatalf("second op: %v", err)
	}
	if err := ctrl.Run(bg, "two"); err != nil {
		t.Fatalf("turn two: %v", err)
	}

	reqs := rec.requests()
	if len(reqs) < 2 {
		t.Fatalf("requests = %d, want at least two turns", len(reqs))
	}
	first, second := reqs[0], reqs[1]

	if got, want := systemText(first), systemText(second); got != want {
		t.Fatalf("the system prefix changed between turns:\n%q\nvs\n%q", got, want)
	}
	if got, want := toolsJSON(t, first), toolsJSON(t, second); got != want {
		t.Fatalf("the tool surface changed between turns:\n%s\nvs\n%s", got, want)
	}

	firstBody := lastTurnText(first)
	if !strings.Contains(firstBody, "node id=first") {
		t.Fatalf("turn one never carried the seeded op:\n%s", firstBody)
	}
	if strings.Contains(firstBody, "node id=second") {
		t.Fatalf("turn one saw an op that did not exist yet:\n%s", firstBody)
	}
	secondBody := lastTurnText(second)
	if !strings.Contains(secondBody, "node id=second") {
		t.Fatalf("turn two never carried the delta:\n%s", secondBody)
	}
	if strings.Contains(secondBody, "node id=first") {
		t.Fatalf("turn two re-sent what turn one already delivered:\n%s", secondBody)
	}
	if !strings.Contains(secondBody, "agentbus view schema=agentbus-view/1") {
		t.Fatalf("the view header must stay byte-identical:\n%s", secondBody)
	}
}

// lastTurnText returns the newest non-system message: that is the turn now being
// asked about, while earlier turns are history the provider must still see.
func lastTurnText(req provider.Request) string {
	out := ""
	for _, message := range req.Messages {
		if message.Role != provider.RoleSystem {
			out = message.Content
		}
	}
	return out
}

func systemText(req provider.Request) string {
	var b strings.Builder
	for _, message := range req.Messages {
		if message.Role == provider.RoleSystem {
			b.WriteString(message.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func toolsJSON(t *testing.T, req provider.Request) string {
	t.Helper()
	raw, err := json.Marshal(req.Tools)
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	return string(raw)
}
