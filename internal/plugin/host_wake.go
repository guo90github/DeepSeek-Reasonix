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

// SetWakeHandler installs the sink for server-initiated wakes. Set it before
// the first connect, like profile: connects read the field without a lock.
// With no handler a wake is logged and dropped rather than delivered in
// silence, so an unwired channel stays visible instead of becoming a second
// delivery path nobody can observe.
func (h *Host) SetWakeHandler(f func(WakeMessage)) {
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
		slog.Warn("plugin: wake notification has no handler; dropped",
			"server", msg.Server, "method", msg.Method)
		return
	}
	h.wakeHandler(msg)
}
