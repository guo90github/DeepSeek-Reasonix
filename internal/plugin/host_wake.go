package plugin

import (
	"encoding/json"
	"log/slog"
)

// WakeMessage is one wake a server sent without being asked: the notification
// its declared Spec.WakeMethod delivered. Whether it becomes work — a queued
// turn, a mid-turn steer — is the host's decision; the server only says that
// something arrived.
type WakeMessage struct {
	Server string
	Method string
	// Caller is the session that last called this server, empty when nothing
	// has: one host serves several sessions, so a wake carries the only
	// identity the child could learn, and the host never guesses one.
	Caller  string
	Payload json.RawMessage
}

// WakeOutcome is what a host did with one wake. The vocabulary lives here so the
// sink's answer, the log line, and diagnostics cannot describe the same wake
// three different ways: a wake nobody could admit must stay distinguishable from
// one that was delivered.
type WakeOutcome string

const (
	WakeDelivered      WakeOutcome = "delivered"
	WakeKeyCollision   WakeOutcome = "key-collision"
	WakeRefused        WakeOutcome = "refused"
	WakeFailed         WakeOutcome = "failed"
	WakeNoHandler      WakeOutcome = "no-handler"
	WakeUnroutable     WakeOutcome = "unroutable"
	WakeUndeliverable  WakeOutcome = "undeliverable"
	WakeOutcomeUnknown WakeOutcome = "unknown"
)

// SetWakeHandler installs the sink for server-initiated wakes. Set it before
// the first connect, like profile: connects read the field without a lock.
// The sink returns what it did with the wake so an undelivered one is reported
// by the host that owns the connection, not only by the sink that tried.
func (h *Host) SetWakeHandler(f func(WakeMessage) WakeOutcome) {
	h.wakeHandler = f
}

// bindWakeNotifications arms the wake method this client's server declared.
// A server that declared none stays unable to wake anything, which is the
// default: connecting is not a grant.
func (h *Host) bindWakeNotifications(c *Client) {
	if h == nil || c == nil {
		return
	}
	server, method := c.name, c.spec.WakeMethod
	c.watchWakeNotifications(func(payload json.RawMessage) {
		h.dispatchWake(WakeMessage{Server: server, Method: method, Caller: c.wakeCaller(), Payload: payload})
	})
}

func (h *Host) dispatchWake(msg WakeMessage) {
	if h == nil {
		return
	}
	if h.wakeHandler == nil {
		h.noteWakeOutcome(msg.Server, WakeNoHandler)
		slog.Warn("plugin: wake notification has no handler; dropped",
			"server", msg.Server, "method", msg.Method, "outcome", string(WakeNoHandler))
		return
	}
	outcome := h.wakeHandler(msg)
	if outcome == "" {
		outcome = WakeOutcomeUnknown
	}
	h.noteWakeOutcome(msg.Server, outcome)
	if outcome != WakeDelivered {
		slog.Warn("plugin: server wake was not delivered",
			"server", msg.Server, "method", msg.Method, "outcome", string(outcome))
	}
}

// noteWakeOutcome remembers the last answer for one server, which is what lets a
// status surface say "this room's last push was refused" instead of nothing.
func (h *Host) noteWakeOutcome(server string, outcome WakeOutcome) {
	if server == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.wakeOutcomes == nil {
		h.wakeOutcomes = map[string]WakeOutcome{}
	}
	h.wakeOutcomes[server] = outcome
}

// WakeOutcomeFor reports what the host last did with a wake from this server,
// empty when it has not woken anything yet.
func (h *Host) WakeOutcomeFor(server string) WakeOutcome {
	if h == nil {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.wakeOutcomes[server]
}
