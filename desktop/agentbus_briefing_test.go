package main

import (
	"context"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

func TestAgentBusBriefingBindingShapesWhatThePanelDraws(t *testing.T) {
	dir := t.TempDir()
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	ctrl.SetAgentBus(dir, "alice")
	if _, err := ctrl.OpenAgentBusHearing(context.Background(), "design", []string{"alice"}); err != nil {
		t.Fatalf("open hearing: %v", err)
	}
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: ctrl}}, activeTabID: "t1"}

	view, err := app.AgentBusBriefing()
	if err != nil {
		t.Fatalf("briefing: %v", err)
	}
	if view.Participant != "alice" {
		t.Fatalf("participant = %q, want the identity the panel shows", view.Participant)
	}
	if len(view.Cards) != 1 || view.Cards[0].Subtree != "design" || view.Cards[0].Disputed != 1 {
		t.Fatalf("cards = %+v, want the subtree under deliberation", view.Cards)
	}
	if len(view.Signals) != 1 || view.Signals[0].Kind != "disputed" {
		t.Fatalf("signals = %+v, want the drill-in row", view.Signals)
	}
	if view.Signals[0].Node != "design" || view.Signals[0].Detail == "" {
		t.Fatalf("signal = %+v, want an address and a reason", view.Signals[0])
	}
}

func TestAgentBusBriefingBindingSaysWhyItHasNothing(t *testing.T) {
	if _, err := (&App{}).AgentBusBriefing(); err == nil {
		t.Fatal("no active session must be reported, not shown as an empty board")
	}
	offBoard := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	app := &App{tabs: map[string]*WorkspaceTab{"t1": {ID: "t1", Ctrl: offBoard}}, activeTabID: "t1"}
	if _, err := app.AgentBusBriefing(); err == nil {
		t.Fatal("a session that never joined a board must say so")
	}
}
