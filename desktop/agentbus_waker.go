package main

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
)

// enrollAgentBus gives a freshly built controller the host's wake routing and lets
// it sweep once: the desktop knows which tab owns which participant, so it — not the
// kernel — is where a wake can be delivered.
func (a *App) enrollAgentBus(ctrl control.SessionAPI) {
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return
	}
	bus.SetAgentBusWaker(a.routeAgentBusWake)
	// Off the build path on purpose: the sweep may enqueue a turn, and a controller
	// that is still being published must not be asked to start one synchronously.
	go bus.WakeAgentBus(context.Background())
}

// routeAgentBusWake hands a wake to the tab that speaks as the participant. The wake
// becomes a durable follow-up keyed by the wake itself, so a repeat collapses and an
// idle session actually gets a turn out of it.
func (a *App) routeAgentBusWake(ctx context.Context, target agentbus.WakeTarget) error {
	tab := a.agentBusWakeRecipient(target.Participant)
	if tab == nil {
		return fmt.Errorf("desktop: no tab owns agentbus participant %q", target.Participant)
	}
	text := agentBusWakePrompt(target)
	_, err := tab.Ctrl.TryEnqueueFollowup(control.InboxRequest{
		Submit:      text,
		Raw:         text,
		Source:      "agentbus",
		Idempotency: target.Key,
	})
	return err
}

// agentBusWakeRecipient finds the tab that owns a participant. It is the whole
// routing decision, kept separate so it can be tested without an inbox.
func (a *App) agentBusWakeRecipient(participant string) *WorkspaceTab {
	if strings.TrimSpace(participant) == "" {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.tabs {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		bus, ok := tab.Ctrl.(control.AgentBusControl)
		if !ok || bus.AgentBusParticipant() != participant {
			continue
		}
		return tab
	}
	return nil
}

// agentBusWakePrompt states why a session is being woken. The view and talk blocks
// ride the same turn, so this only has to carry the reason.
func agentBusWakePrompt(target agentbus.WakeTarget) string {
	var b strings.Builder
	b.WriteString("<agentbus-wake>\n")
	b.WriteString("The board woke you: it has work only you can move right now.\n")
	writeWakeList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.Join(items, ", "))
		b.WriteString("\n")
	}
	writeWakeList("startable now", target.Ready)
	writeWakeList("waiting on you", target.Waiting)
	writeWakeList("questions addressed to you", target.Asks)
	writeWakeList("deliberations you owe an answer about", target.Owes)
	b.WriteString("</agentbus-wake>\n")
	return b.String()
}
