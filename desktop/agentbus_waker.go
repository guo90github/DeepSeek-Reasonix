package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"reasonix/internal/agentbus"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

// agentBusWakeLedger is one ledger for this host process: every controller the desktop
// rebuilds — a join, a tab switch, a settings change — shares it, so the same work set
// is not delivered to a participant twice.
var agentBusWakeLedger = control.NewWakeLedger()

// agentBusBudget is this host's spending account, shared for the same reason the wake
// ledger is: the ceilings belong to the machine.
//
// The ceilings come from [agentbus] in config, read once per process. One account per host is
// what keeps a settings change — which rebuilds controllers — from forgiving what was already
// spent; the price is that a change needs a restart.
var (
	agentBusBudgetOnce sync.Once
	agentBusBudget     *agentbus.Ledger
	agentBusBrake      bool
)

func hostAgentBusBudget() *agentbus.Ledger {
	agentBusBudgetOnce.Do(func() {
		cfg := agentBusConfig()
		agentBusBudget = agentbus.NewLedger(agentBusBudgetLimits(cfg))
		agentBusBrake = agentBusBudgetBrake(cfg)
	})
	return agentBusBudget
}

// hostAgentBusBudgetBrake reports whether the account above has any spending ceiling. It reads
// through that account, so the answer can never describe a brake the enforcing ledger lacks.
func hostAgentBusBudgetBrake() bool {
	hostAgentBusBudget()
	return agentBusBrake
}

// agentBusBudgetBrake is true when any of the four spending levels carries a limit. The slot
// ceiling bounds how many sessions work at once, not how far the work can go, so it is not a
// brake on cost — and an operator whose host has none cannot see that anywhere else (2026-10-03).
func agentBusBudgetBrake(cfg config.AgentBusConfig) bool {
	return cfg.BudgetBoard > 0 || cfg.BudgetSubtree > 0 || cfg.BudgetNode > 0 || cfg.BudgetTurn > 0
}

// agentBusBudgetLimits maps the operator's knobs onto the kernel's four levels plus the host's
// slot ceiling. Zero stays zero: the kernel invents no ceilings on the operator's behalf.
func agentBusBudgetLimits(cfg config.AgentBusConfig) agentbus.BudgetLimits {
	return agentbus.BudgetLimits{
		Board:   cfg.BudgetBoard,
		Subtree: cfg.BudgetSubtree,
		Node:    cfg.BudgetNode,
		Turn:    cfg.BudgetTurn,
		Slots:   cfg.DispatchSlots,
	}
}

func agentBusConfig() config.AgentBusConfig {
	cfg, err := config.Load()
	if err != nil {
		// A config that cannot be read leaves every ceiling at zero, which is the behavior of
		// a host nobody configured.
		return config.AgentBusConfig{}
	}
	return cfg.AgentBus
}

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
//
// A session in the middle of a turn is left alone: the wake would wait in its guidance
// queue while the lease ran, holding the step for half an hour with nobody looking at it,
// and the next tick offers it again anyway (found on a real machine, 2026-10-03).
func (a *App) agentBusDispatchTick() {
	a.mu.RLock()
	tabs := make([]*WorkspaceTab, 0, len(a.tabs))
	for _, tab := range a.tabs {
		if tab != nil && tab.Ctrl != nil {
			tabs = append(tabs, tab)
		}
	}
	a.mu.RUnlock()

	done := map[string]bool{}
	for _, tab := range tabs {
		bus, ok := tab.Ctrl.(control.AgentBusControl)
		if !ok {
			continue
		}
		participant := bus.AgentBusParticipant()
		boardDir := bus.AgentBusDir()
		if strings.TrimSpace(participant) == "" || strings.TrimSpace(boardDir) == "" {
			continue
		}
		key := boardDir + "\x00" + participant
		if done[key] {
			continue
		}
		done[key] = true
		// Outside the lock on purpose: ActiveWorkForTab reads the tab table again, and a
		// recursive read lock can deadlock behind a waiting writer.
		if a.ActiveWorkForTab(tab.ID).active() {
			continue
		}
		if _, err := bus.AgentBusDispatch(context.Background(), participant,
			func(ctx context.Context, target agentbus.WakeTarget) error {
				return a.routeAgentBusWakeOn(ctx, boardDir, target)
			}); err != nil {
			log.Printf("[agentbus] dispatch to %s: %v", participant, err)
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
	bus.SetAgentBusLedger(hostAgentBusBudget())
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
	tab, err := a.agentBusWakeRecipient(target.Participant)
	if err != nil {
		return err
	}
	if tab != nil {
		return enqueueAgentBusWake(tab, target)
	}
	return a.deliverAgentBusWakeRemotely(ctx, boardDir, target)
}

// deliverAgentBusWakeRemotely asks the board's address book where this participant
// speaks from and posts the wake there with that host's token. No address is a
// refusal, never a drop: a wake nobody receives must be visible to its sender.
func (a *App) deliverAgentBusWakeRemotely(ctx context.Context, boardDir string, target agentbus.WakeTarget) error {
	if strings.TrimSpace(boardDir) == "" {
		return agentbus.NoRoute(target.Participant)
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
		return agentbus.NoRoute(target.Participant)
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
	return control.AgentBusWakeLine(target)
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
//
// Two tabs speaking as one participant is an error, not a coin toss: waking the wrong session
// is the failure this routing exists to prevent, and serve reports the same ambiguity
// (internal/serve/agentbus_waker.go). "别搞错会话" is a criterion, not a nicety.
func (a *App) agentBusWakeRecipient(participant string) (*WorkspaceTab, error) {
	if strings.TrimSpace(participant) == "" {
		return nil, nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	var found *WorkspaceTab
	for _, tab := range a.tabs {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		bus, ok := tab.Ctrl.(control.AgentBusControl)
		if !ok || bus.AgentBusParticipant() != participant {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("desktop: two tabs speak as agentbus participant %q", participant)
		}
		found = tab
	}
	return found, nil
}

// agentBusWakePrompt renders the block a woken session reads. The rendering itself lives in
// control so the injection site can rebuild it from the same target
// (control.AgentBusWakePrompt); this host-side name exists for the call sites and tests that
// were written against it, and goes away with the injection-time rebuild.
func agentBusWakePrompt(target agentbus.WakeTarget) string {
	return control.AgentBusWakePrompt(target)
}
