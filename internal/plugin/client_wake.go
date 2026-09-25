package plugin

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// watchWakeNotifications routes the one wake method this server declared to
// sink. An empty Spec.WakeMethod leaves it unarmed: a server cannot wake this
// session unless the configuration says it may.
func (c *Client) watchWakeNotifications(sink func(json.RawMessage)) {
	if c == nil || sink == nil {
		return
	}
	method := strings.TrimSpace(c.spec.WakeMethod)
	if method == "" {
		return
	}
	t, ok := c.t.(notificationTransport)
	if !ok {
		slog.Warn("plugin: transport cannot receive wake notifications; server cannot wake this session",
			"server", c.spec.Name, "method", method)
		return
	}
	stop := t.registerNotification(method, sink)
	c.surfaceStopsMu.Lock()
	if c.closed.Load() {
		c.surfaceStopsMu.Unlock()
		stop()
		return
	}
	c.surfaceStops = append(c.surfaceStops, stop)
	c.surfaceStopsMu.Unlock()
}
