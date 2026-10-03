package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
)

// agentBusWakeLedger is one ledger for this host process: every controller the desktop
// rebuilds — a join, a tab switch, a settings change — shares it, so the same work set
// is not delivered to a participant twice.
var agentBusWakeLedger = control.NewWakeLedger()

// agentBusBudget is this host's spending account, shared for the same reason the wake
// ledger is: the ceilings belong to the machine. Zero limits mean the kernel invents no
// ceiling — an operator sets them, not this host.
var agentBusBudget = agentbus.NewLedger(agentbus.BudgetLimits{})

// agentBusWakeTick is the board's share of the host tick: every enrolled controller
// reclaims what a lapsed lease left behind and wakes whoever was waiting on it. One
// controller per tab runs it, so a board several tabs joined is swept several times —
// each sweep is idempotent and writes nothing once nothing has lapsed.
func (a *App) agentBusWakeTick() {
	a.mu.RLock()
	tabs := make([]*WorkspaceTab, 0, len(a.tabs))
	for _, tab := range a.tabs {
		tabs = append(tabs, tab)
	}
	a.mu.RUnlock()
	for _, tab := range tabs {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		bus, ok := tab.Ctrl.(control.AgentBusControl)
		if !ok {
			continue
		}
		bus.AgentBusTick(context.Background())
	}
	a.agentBusDispatchTick()
}

// agentBusDispatchTick hands startable work to the participants this host owns, one step
// each per tick, and delivers every assignment to that participant's own session — the
// claimant is named, and the routing resolves it to a tab or an announced address, so a
// participant is never given work on the strength of a guess.
func (a *App) agentBusDispatchTick() {
	type enrolled struct {
		ctrl        control.AgentBusControl
		boardDir    string
		participant string
	}
	a.mu.RLock()
	all := make([]enrolled, 0, len(a.tabs))
	for _, tab := range a.tabs {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		bus, ok := tab.Ctrl.(control.AgentBusControl)
		if !ok || strings.TrimSpace(bus.AgentBusDir()) == "" {
			continue
		}
		all = append(all, enrolled{ctrl: bus, boardDir: bus.AgentBusDir(), participant: bus.AgentBusParticipant()})
	}
	a.mu.RUnlock()

	done := map[string]bool{}
	for _, host := range all {
		if strings.TrimSpace(host.participant) == "" {
			continue
		}
		key := host.boardDir + "\x00" + host.participant
		if done[key] {
			continue
		}
		done[key] = true
		boardDir := host.boardDir
		if _, err := host.ctrl.AgentBusDispatch(context.Background(), host.participant,
			func(ctx context.Context, target agentbus.WakeTarget) error {
				return a.routeAgentBusWakeOn(ctx, boardDir, target)
			}); err != nil {
			log.Printf("[agentbus] dispatch to %s: %v", host.participant, err)
		}
	}
}

// enrollAgentBus gives a freshly built controller the host's wake routing and lets
// it sweep once: the desktop knows which tab owns which participant, so it — not the
// kernel — is where a wake can be delivered.
func (a *App) enrollAgentBus(ctrl control.SessionAPI) {
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return
	}
	boardDir := bus.AgentBusDir()
	bus.SetAgentBusWakeLedger(agentBusWakeLedger)
	bus.SetAgentBusLedger(agentBusBudget)
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
		Display:     agentBusWakeLine(target),
		Source:      "agentbus",
		Idempotency: target.Key,
	})
	return err
}

// agentBusWakeLine says the same thing to a person: the block above is what the model reads,
// and the queue that shows this to whoever is looking must not render XML at them.
func agentBusWakeLine(target agentbus.WakeTarget) string {
	var b strings.Builder
	b.WriteString("The board has work for you")
	appendList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("; ")
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.Join(items, ", "))
	}
	appendList("startable now", target.Ready)
	appendList("waiting on you", target.Waiting)
	appendList("questions for you", target.Asks)
	appendList("deliberations you owe", target.Owes)
	return b.String()
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
