package cli

import (
	"context"
	"strings"
	"sync"
	"time"

	"reasonix/internal/boot"
	"reasonix/internal/control"
)

// agentBusTickInterval is how often a headless host advances its board.
//
// A serve host has no turn loop of its own, so nothing else would reclaim a lapsed lease or
// renew its address: the board would sit still until somebody wrote to it. Found on a real
// machine, where two dispatched leases went untouched for 27 minutes (2026-10-03).
const agentBusTickInterval = 30 * time.Second

// startAgentBusTick advances the board on a timer and returns the function that stops it.
//
// The address is republished on every tick: an announcement is how another host finds this
// one, and one that is never renewed goes stale (AGENT_BUS §11.5).
func startAgentBusTick(ctrl *control.Controller, opts boot.Options, interval time.Duration) func() {
	if ctrl == nil || strings.TrimSpace(opts.AgentBusDir) == "" {
		return func() {}
	}
	if interval <= 0 {
		interval = agentBusTickInterval
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				ctrl.AgentBusTick(context.Background())
				announceAgentBusSession(ctrl, opts)
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}
