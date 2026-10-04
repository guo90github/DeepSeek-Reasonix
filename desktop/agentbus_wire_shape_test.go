package main

import (
	"encoding/json"
	"strings"

	"reasonix/internal/agentbus/board"
	"testing"
)

// The panel iterates these lists on every render, so the wire contract has to carry
// arrays even when the board is empty: a Go nil slice marshals to null, and the
// frontend's "[...view.cards]" then throws "cards is not iterable" — the whole app
// went down on a session that had joined a board nobody had written to yet.
func TestAgentBusBriefingCarriesEmptyListsNotNull(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	// An empty board: enrolled, no cards, no signals. This is the state that crashed.
	boardDir := t.TempDir()
	ctrl.SetAgentBus(boardDir, "alice")

	view, err := app.AgentBusBriefing()
	if err != nil {
		t.Fatalf("briefing: %v", err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The host's own rows — a refused claim, a wake that reached nobody — are process-wide, so an
	// empty board may still carry one. What this test pins is the shape the panel iterates
	// (arrays, never null), not how many rows a host has accumulated.
	for _, want := range []string{`"cards":[]`, `"signals":[`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("briefing JSON = %s, want %s", raw, want)
		}
	}
	if strings.Contains(string(raw), `"cards":null`) || strings.Contains(string(raw), `"signals":null`) {
		t.Fatalf("briefing JSON carries null lists the panel cannot iterate: %s", raw)
	}
}

// Same contract for one step's record: deps, refutations and authorizations are
// iterated too.
func TestAgentBusNodeDetailCarriesEmptyListsNotNull(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	boardDir := t.TempDir()
	ctrl.SetAgentBus(boardDir, "alice")
	if _, err := ctrl.ApplyAgentBusOp(t.Context(), board.Op{
		Verb: board.VerbAssert, Node: "publish", Actor: "alice",
		Evidence: []board.Evidence{{Kind: "test", Ref: "go test ./..."}},
	}); err != nil {
		t.Fatalf("assert: %v", err)
	}

	view, err := app.AgentBusNodeDetail("publish")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"deps":[]`, `"refutations":[]`, `"authorizations":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("detail JSON = %s, want %s", raw, want)
		}
	}
}
