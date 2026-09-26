package boot

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
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
		body, extra, err := wakeInbox(msg)
		if err != nil {
			slog.Warn("boot: server wake has no usable body; dropped", "server", msg.Server, "err", err)
			return
		}
		req := control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: body, Display: body, Raw: body,
			Extra: extra,
		}
		if _, err := ctrl.EnqueueInbox(req); err != nil {
			slog.Warn("boot: server wake could not be queued", "server", msg.Server, "err", err)
		}
	}
}

// wakeInbox reads the guidance out of a wake payload. The sending side already
// decided this line addresses this session, so the host neither re-derives
// mentions nor rewrites the text it will show; the structured room fields are
// carried verbatim for the frontend to badge.
func wakeInbox(msg plugin.WakeMessage) (string, map[string]string, error) {
	var payload struct {
		Text     string   `json:"text"`
		Seq      int64    `json:"seq"`
		From     string   `json:"from"`
		Topic    int64    `json:"topic"`
		Kind     string   `json:"kind"`
		Origin   string   `json:"origin"`
		Mentions []string `json:"mentions"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return "", nil, err
	}
	body := strings.TrimSpace(payload.Text)
	if body == "" {
		return "", nil, errors.New("wake payload carries no text")
	}
	extra := map[string]string{}
	if payload.Seq > 0 {
		extra["room.seq"] = strconv.FormatInt(payload.Seq, 10)
	}
	if payload.From != "" {
		extra["room.from"] = payload.From
	}
	if payload.Topic > 0 {
		extra["room.topic"] = strconv.FormatInt(payload.Topic, 10)
	}
	if payload.Kind != "" {
		extra["room.kind"] = payload.Kind
	}
	if payload.Origin != "" {
		extra["room.origin"] = payload.Origin
	}
	if len(payload.Mentions) > 0 {
		extra["room.mentions"] = strings.Join(payload.Mentions, "\n")
	}
	if len(extra) == 0 {
		extra = nil
	}
	return body, extra, nil
}
