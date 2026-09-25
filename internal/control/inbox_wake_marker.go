package control

import (
	"strings"

	"reasonix/internal/sessioninbox"
)

// inboxGuidanceSources is the closed set a wake marker may print a source from.
// A producer this list does not name yet still gets a marker — as "unknown":
// swallowing it would leave a reader to conclude the guidance was queued here.
var inboxGuidanceSources = map[string]string{
	"http": "http",
	"push": "push",
	"acp":  "acp",
	"bot":  "bot",
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
	return inboxWakeMarkerPrefix + source + " item=" + meta.ID + "]"
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
