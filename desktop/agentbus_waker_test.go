package main

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

func agentBusTab(t *testing.T, dir, id, participant string) *WorkspaceTab {
	t.Helper()
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	ctrl.SetAgentBus(dir, participant)
	return &WorkspaceTab{ID: id, Ctrl: ctrl}
}

func TestAgentBusWakeRecipientFindsTheOwningTab(t *testing.T) {
	dir := t.TempDir()
	app := &App{tabs: map[string]*WorkspaceTab{
		"t1": agentBusTab(t, dir, "t1", "alice"),
		"t2": agentBusTab(t, dir, "t2", "bob"),
	}}

	if tab := app.agentBusWakeRecipient("bob"); tab == nil || tab.ID != "t2" {
		t.Fatalf("recipient = %+v, want the tab that speaks as bob", tab)
	}
	if tab := app.agentBusWakeRecipient("nobody"); tab != nil {
		t.Fatalf("recipient = %+v, want none for a participant no tab owns", tab)
	}
	if tab := app.agentBusWakeRecipient("   "); tab != nil {
		t.Fatal("an empty participant must route to nobody rather than to the first tab")
	}
}

func TestRouteAgentBusWakeReportsAnUnreachableParticipant(t *testing.T) {
	app := &App{tabs: map[string]*WorkspaceTab{}}
	err := app.routeAgentBusWake(context.Background(), agentbus.WakeTarget{Participant: "ghost", Key: "k"})
	if err == nil {
		t.Fatal("a wake no tab owns must be reported, not dropped silently")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("err = %v, want it to name the participant", err)
	}
}

func TestAgentBusWakePromptCarriesTheReason(t *testing.T) {
	prompt := agentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob",
		Ready:       []string{"schema"},
		Waiting:     []string{"design"},
		Asks:        []string{"ask-1"},
		Owes:        []string{"dispute"},
	})
	for _, want := range []string{"<agentbus-wake>", "schema", "design", "ask-1", "dispute"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt is missing %q:\n%s", want, prompt)
		}
	}
	if empty := agentBusWakePrompt(agentbus.WakeTarget{Participant: "bob"}); strings.Contains(empty, "startable now") {
		t.Fatalf("a wake with nothing to show must not invent a list:\n%s", empty)
	}
}
