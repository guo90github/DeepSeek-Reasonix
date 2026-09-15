package agentd

import (
	"context"
	"time"
)

const (
	backoffBase    = time.Second
	backoffMax     = 30 * time.Second
	crashWindow    = 10 * time.Minute
	crashLimit     = 5
	circuitOpenFor = 10 * time.Minute
)

// watchState is one instance's restart bookkeeping. It is grouped by lifetime
// rather than spread over the manager, which keeps the manager's own guarded
// fields countable.
type watchState struct {
	attempts    int
	windowStart time.Time
	openedAt    time.Time
}

// Watch probes every managed instance on an interval and restarts the ones that
// died. A crash loop opens a circuit instead of retrying forever.
func (m *Manager) Watch(ctx context.Context) error {
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := m.Sweep(ctx); err != nil {
				return err
			}
		}
	}
}

// Sweep is one supervision pass: refresh every record, then restart what is
// down unless its circuit is open.
func (m *Manager) Sweep(ctx context.Context) error {
	reg, err := LoadRegistry(m.registry)
	if err != nil {
		return err
	}
	refreshed, err := m.refresh(ctx, reg)
	if err != nil {
		return err
	}
	for _, rec := range refreshed.Instances {
		if rec.State == StateRunning || m.circuitOpen(rec.Name) {
			continue
		}
		m.restart(ctx, rec)
	}
	return nil
}

// restart relaunches one instance, or opens its circuit once the crash window
// has had enough attempts.
func (m *Manager) restart(ctx context.Context, rec Record) {
	m.mu.Lock()
	state := m.watch[rec.Name]
	if state == nil {
		state = &watchState{windowStart: m.now()}
		m.watch[rec.Name] = state
	}
	if m.now().Sub(state.windowStart) > crashWindow {
		state.attempts = 0
		state.windowStart = m.now()
	}
	if state.attempts >= crashLimit {
		state.openedAt = m.now()
		opened := rec
		opened.State = StateCircuitOpen
		m.mu.Unlock()
		_ = m.save(func(reg Registry) Registry { return reg.Upsert(opened) })
		return
	}
	state.attempts++
	delay := m.backoff(state.attempts)
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}
	managed, err := Spawn(ctx, m.specFor(rec))
	if err != nil {
		return
	}
	if err := m.awaitReady(ctx, managed); err != nil {
		managed.Stop()
		return
	}
	m.mu.Lock()
	if previous := m.live[rec.Name]; previous != nil {
		previous.closeLog()
	}
	m.live[rec.Name] = managed
	m.mu.Unlock()
	respawned := managed.Record
	respawned.LastHealthy = m.now()
	_ = m.save(func(reg Registry) Registry { return reg.Upsert(respawned) })
}

// backoffDelay grows 1s, 2s, 4s … up to backoffMax.
func backoffDelay(attempt int) time.Duration {
	delay := backoffBase
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= backoffMax {
			return backoffMax
		}
	}
	return delay
}

// circuitOpen reports whether restarts for this instance are parked. The
// decision is time-derived, so it survives a manager restart.
func (m *Manager) circuitOpen(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.watch[name]
	if state == nil || state.openedAt.IsZero() {
		return false
	}
	return m.now().Sub(state.openedAt) < circuitOpenFor
}
