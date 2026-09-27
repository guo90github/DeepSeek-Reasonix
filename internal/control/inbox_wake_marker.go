package control

import (
	"strconv"
	"strings"

	"reasonix/internal/sessioninbox"
)

// inboxGuidanceSources is the closed set a wake marker may print a source from.
// A producer this list does not name yet still gets a marker — as "unknown":
// swallowing it would leave a reader to conclude the guidance was queued here.
var inboxGuidanceSources = map[string]string{
	"http":      "http",
	"push":      "push",
	"acp":       "acp",
	"bot":       "bot",
	"room-wake": "room-wake",
}

// WakeSourceRoom is the source a wake carries when the chat room's own line
// number identifies it — the transport that carried it is then not the story.
const WakeSourceRoom = "room-wake"

// WakeSourceFor picks the provenance a wake reports: an item naming a room line
// is a room wake, anything else stays the transport it arrived on. Both ingest
// routes call it, so the marker's vocabulary has one author instead of two.
func WakeSourceFor(extra map[string]string, transport string) string {
	if extra["room.seq"] != "" {
		return WakeSourceRoom
	}
	return transport
}

const (
	inboxWakeMarkerPrefix  = "[remote wake source="
	inboxWakeSourceUnknown = "unknown"
)

// inboxWakeMarker names where guidance pushed in from outside this session came
// from. Without it the injected wrapper says "queued by the user" and a woken
// session reports never having been woken (measured 2026-09-25: a steer that
// arrived three minutes late was read as its owner typing). An item with no
// source was queued in this process, where that wrapper is already truthful.
func inboxWakeMarker(meta sessioninbox.InboxItemMeta) string {
	source := strings.TrimSpace(meta.Source)
	if source == "" || strings.TrimSpace(meta.ID) == "" {
		return ""
	}
	if named, ok := inboxGuidanceSources[source]; ok {
		source = named
	} else {
		source = inboxWakeSourceUnknown
	}
	return inboxWakeMarkerPrefix + source + " item=" + meta.ID + roomSeqSuffix(meta) + "]"
}

// roomSeqSuffix prints the room line's own seq — the one number both sides can
// name — beside the host-only item id, so a reader reconciles a wake against the
// line that caused it. An item with no room provenance prints none: a guessed
// seq would be worse than its absence.
func roomSeqSuffix(meta sessioninbox.InboxItemMeta) string {
	if meta.Room == nil || meta.Room.Seq <= 0 {
		return ""
	}
	return " seq=" + strconv.FormatInt(meta.Room.Seq, 10)
}

// markInboxGuidance puts the marker on the guidance the model will see. It
// rides the message body — the turn tail — because the system-prompt prefix has
// to stay byte-identical across turns for the provider's prefix cache.
func markInboxGuidance(meta sessioninbox.InboxItemMeta, text string) string {
	marker := inboxWakeMarker(meta)
	if marker == "" || strings.TrimSpace(text) == "" {
		return text
	}
	return marker + "\n" + text
}
