package control

import (
	"sync"
	"time"
)

// dispatchRefusalCooldown is how long one budget refusal keeps a step out of the dispatch loop.
// Retrying every host tick produced a WARN + INFO pair every 30s for as long as the ceiling
// stood, with no upper bound (measured 2026-10-04: refusals=1…11 in six minutes, node unchanged).
const dispatchRefusalCooldown = 5 * time.Minute

// dispatchRefusalMemo remembers steps a spending ceiling turned down, so this session stops
// handing them out until the cooldown lapses. It travels with the state that measured the refusal,
// so another session's or account's refusals cannot reach inside it — one memo for the whole
// process leaked one test's refusals into another's work (2026-10-05) — and it prunes expired
// entries as it writes, so it cannot grow with the board.
type dispatchRefusalMemo struct {
	mu    sync.Mutex
	until map[string]time.Time
}

// refusalKey names one step. The node is the whole key: the taker at the charge and the requester
// at the park are different participants, so a claimant-qualified key never matched itself.
func refusalKey(_ string, node string) string { return node }

// skip reports whether this step is still inside its refusal cooldown.
func (m *dispatchRefusalMemo) skip(claimant, node string) bool {
	if m == nil || node == "" {
		return false
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	key := refusalKey(claimant, node)
	at, held := m.until[key]
	if !held {
		return false
	}
	if now.After(at) {
		delete(m.until, key)
		return false
	}
	return true
}

// note starts (or restarts) one step's cooldown.
func (m *dispatchRefusalMemo) note(claimant, node string) {
	if m == nil || node == "" {
		return
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.until == nil {
		m.until = map[string]time.Time{}
	}
	for held, at := range m.until {
		if now.After(at) {
			delete(m.until, held)
		}
	}
	m.until[refusalKey(claimant, node)] = now.Add(dispatchRefusalCooldown)
}
