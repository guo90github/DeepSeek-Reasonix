package boot

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"reasonix/internal/control"
	"reasonix/internal/plugin"
	"reasonix/internal/sessioninbox"
)

// installInboxWakeHandler points server-initiated wakes at the session this
// host serves, so a mention lands as guidance even while that session is idle
// or mid-turn — the point of not depending on a poll being in flight. A shared
// host serves several sessions and has no such binding.
func installInboxWakeHandler(host *plugin.Host, ctrl *control.Controller) {
	if host == nil || ctrl == nil {
		return
	}
	host.SetWakeHandler(inboxWakeHandler(ctrl))
}

func inboxWakeHandler(ctrl *control.Controller) func(plugin.WakeMessage) {
	if ctrl == nil {
		return nil
	}
	return func(msg plugin.WakeMessage) {
		body, err := wakeInboxBody(msg)
		if err != nil {
			slog.Warn("boot: server wake has no usable body; dropped", "server", msg.Server, "err", err)
			return
		}
		req := control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: body, Display: body, Raw: body,
		}
		if _, err := ctrl.EnqueueInbox(req); err != nil {
			slog.Warn("boot: server wake could not be queued", "server", msg.Server, "err", err)
		}
	}
}

// wakeInboxBody reads the guidance out of a wake payload. The sending side
// already decided this line addresses this session, so the host neither
// re-derives mentions nor rewrites the text it will show.
func wakeInboxBody(msg plugin.WakeMessage) (string, error) {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return "", err
	}
	body := strings.TrimSpace(payload.Text)
	if body == "" {
		return "", errors.New("wake payload carries no text")
	}
	return body, nil
}
