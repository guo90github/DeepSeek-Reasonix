package boot

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"reasonix/internal/control"
	"reasonix/internal/plugin"
	"reasonix/internal/sessioninbox"
)

// installInboxWakeHandler points server-initiated wakes at the session this
// host serves, so a mention lands as guidance even while that session is idle
// or mid-turn — the point of not depending on a poll being in flight.
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
	return func(msg plugin.WakeMessage) { deliverInboxWake(ctrl, msg) }
}

// SharedWakeHandler is the sink for a host shared across sessions, which is the
// desktop's shape: the wake lands in the inbox of the session the child last saw
// call it. An unknown caller keeps the visible drop, because a host serving
// several sessions that guessed one would deliver a mention into the wrong inbox.
func SharedWakeHandler(resolve func(string) *control.Controller) func(plugin.WakeMessage) {
	if resolve == nil {
		return nil
	}
	return func(msg plugin.WakeMessage) {
		ctrl := resolve(strings.TrimSpace(msg.Caller))
		if ctrl == nil {
			slog.Warn("boot: server wake names no reachable session; dropped",
				"server", msg.Server, "caller", msg.Caller)
			return
		}
		deliverInboxWake(ctrl, msg)
	}
}

// wakeEnqueueOutcome names what happened to one wake, because the failures are
// not interchangeable: a key already held by another line means this wake was
// dropped, while a full or paused inbox means the same wake may land later.
type wakeEnqueueOutcome int

const (
	wakeEnqueueAccepted wakeEnqueueOutcome = iota
	wakeEnqueueKeyCollision
	wakeEnqueueRefused
	wakeEnqueueFailed
)

func (o wakeEnqueueOutcome) String() string {
	switch o {
	case wakeEnqueueAccepted:
		return "accepted"
	case wakeEnqueueKeyCollision:
		return "key-collision"
	case wakeEnqueueRefused:
		return "refused"
	default:
		return "failed"
	}
}

func wakeEnqueueOutcomeFor(err error) wakeEnqueueOutcome {
	switch {
	case err == nil:
		return wakeEnqueueAccepted
	case errors.Is(err, sessioninbox.ErrIdempotencyConflict):
		return wakeEnqueueKeyCollision
	case errors.Is(err, sessioninbox.ErrPaused),
		errors.Is(err, sessioninbox.ErrCapacityItems),
		errors.Is(err, sessioninbox.ErrCapacityBytes):
		return wakeEnqueueRefused
	default:
		return wakeEnqueueFailed
	}
}

func deliverInboxWake(ctrl *control.Controller, msg plugin.WakeMessage) wakeEnqueueOutcome {
	body, extra, idem, err := wakeInbox(msg)
	if err != nil {
		slog.Warn("boot: server wake has no usable body; dropped", "server", msg.Server, "err", err)
		return wakeEnqueueFailed
	}
	req := control.InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: "push",
		Submit: body, Display: body, Raw: body,
		Extra: extra, Idempotency: idem,
	}
	_, err = ctrl.EnqueueInbox(req)
	outcome := wakeEnqueueOutcomeFor(err)
	switch outcome {
	case wakeEnqueueAccepted:
	case wakeEnqueueKeyCollision:
		// Another line already holds this key: this wake was dropped, and the key
		// is the only evidence that a room identity got reused.
		slog.Warn("boot: server wake collided with another line already queued under its key; dropped",
			"server", msg.Server, "key", idem, "err", err)
	case wakeEnqueueRefused:
		slog.Warn("boot: server wake refused by the inbox; it stays undelivered until the inbox accepts again",
			"server", msg.Server, "key", idem, "err", err)
	default:
		slog.Warn("boot: server wake could not be queued", "server", msg.Server, "key", idem, "err", err)
	}
	return outcome
}

// wakeInbox reads the guidance out of a wake payload. The sending side already
// decided this line addresses this session, so the host neither re-derives
// mentions nor rewrites the text it will show; the structured room fields are
// carried verbatim for the frontend to badge. The idempotency key pins one
// room line to one inbox item so a retried wake cannot queue twice.
func wakeInbox(msg plugin.WakeMessage) (string, map[string]string, string, error) {
	var payload struct {
		Text     string   `json:"text"`
		Seq      int64    `json:"seq"`
		From     string   `json:"from"`
		Topic    int64    `json:"topic"`
		Kind     string   `json:"kind"`
		Origin   string   `json:"origin"`
		Mentions []string `json:"mentions"`
		Panel    string   `json:"panel"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return "", nil, "", err
	}
	body := strings.TrimSpace(payload.Text)
	if body == "" {
		return "", nil, "", errors.New("wake payload carries no text")
	}
	idem := ""
	if msg.Server != "" && payload.Seq > 0 {
		idem = "room-wake:" + msg.Server + ":" + strconv.FormatInt(payload.Seq, 10)
		if u, err := url.Parse(strings.TrimSpace(payload.Panel)); err == nil && u.Host != "" {
			idem = "room-wake:" + msg.Server + ":" + u.Host + ":" + strconv.FormatInt(payload.Seq, 10)
		}
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
	if panel := strings.TrimSpace(payload.Panel); panel != "" && isHTTPPanelURL(panel) {
		extra["room.panel"] = panel
	}
	if len(extra) == 0 {
		extra = nil
	}
	return body, extra, idem, nil
}

// isHTTPPanelURL keeps the open-in-browser link a link: only http(s) URLs with
// a host survive, so a misbehaving server cannot hand the frontend a
// javascript:/file: URL to open.
func isHTTPPanelURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
