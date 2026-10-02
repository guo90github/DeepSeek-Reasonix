package main

import (
	"context"
	"fmt"
	"os"
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
	boardDir := bus.AgentBusDir()
	bus.SetAgentBusWaker(func(ctx context.Context, target agentbus.WakeTarget) error {
		return a.routeAgentBusWakeOn(ctx, boardDir, target)
	})
	// Off the build path on purpose: the sweep may enqueue a turn, and a controller
	// that is still being published must not be asked to start one synchronously.
	go bus.WakeAgentBus(context.Background())
}

// routeAgentBusWake hands a wake to the tab that speaks as the participant. The wake
// becomes a durable follow-up keyed by the wake itself, so a repeat collapses and an
// idle session actually gets a turn out of it.
func (a *App) routeAgentBusWakeOn(ctx context.Context, boardDir string, target agentbus.WakeTarget) error {
	if tab := a.agentBusWakeRecipient(target.Participant); tab != nil {
		return enqueueAgentBusWake(tab, target)
	}
	return a.deliverAgentBusWakeRemotely(ctx, boardDir, target)
}

// deliverAgentBusWakeRemotely asks the board's address book where this participant
// speaks from and posts the wake there with that host's token. No address is a
// refusal, never a drop: a wake nobody receives must be visible to its sender.
func (a *App) deliverAgentBusWakeRemotely(ctx context.Context, boardDir string, target agentbus.WakeTarget) error {
	if strings.TrimSpace(boardDir) == "" {
		return fmt.Errorf("desktop: no board to route the wake for %q", target.Participant)
	}
	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		return err
	}
	ref, ok, err := directory.Lookup(target.Participant)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("desktop: no tab and no address owns agentbus participant %q", target.Participant)
	}
	token, err := readAgentBusToken(ref.TokenFile)
	if err != nil {
		return err
	}
	return deliverAgentBusWake(ctx, nil, AgentBusDelivery{
		BaseURL:       ref.Host,
		Token:         token,
		SessionHeader: agentBusSessionHeader,
		SessionPath:   ref.SessionPath,
	}, target)
}

func enqueueAgentBusWake(tab *WorkspaceTab, target agentbus.WakeTarget) error {
	text := agentBusWakePrompt(target)
	_, err := tab.Ctrl.TryEnqueueFollowup(control.InboxRequest{
		Submit:      text,
		Raw:         text,
		Source:      "agentbus",
		Idempotency: target.Key,
	})
	return err
}

// readAgentBusToken reads the token file a host announced: the secret itself never
// travels in the address book, only where to find it.
func readAgentBusToken(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("desktop: the announced address carries no token file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("desktop: read agentbus token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
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
