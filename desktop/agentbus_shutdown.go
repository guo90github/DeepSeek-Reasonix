package main

import (
	"log/slog"

	"reasonix/internal/control"
)

// withdrawAgentBusOnShutdown retires a session's address on its board as the host goes away.
//
// The board addresses participants by announcement and nothing retires it for a host that
// is gone: a leftover headless serve stayed addressable for a day on a real machine, where
// a wake sent to it was accepted and dropped (2026-10-03). Only the host can do this — the
// address names a process, not a session file, so the board cannot infer the process ended.
func (a *App) withdrawAgentBusOnShutdown(ctrl control.SessionAPI) {
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok || !bus.AgentBusEnrolled() {
		return
	}
	if err := bus.AgentBusWithdraw(); err != nil {
		slog.Warn("desktop: shutdown board withdraw failed", "err", err)
	}
}
