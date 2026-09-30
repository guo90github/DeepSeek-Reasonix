package recap

import (
	"encoding/json"
	"strings"
)

// The four dimensions v1 distills, each a different kind of reuse: a fact
// answers "where is it", a root cause "why did it break", a refutation "do not
// propose this again", a handoff "what is still open".
const (
	KindFact      = "fact"
	KindRootCause = "root-cause"
	KindRefuted   = "refuted"
	KindHandoff   = "handoff"
)

// SinkMemory is where an accepted experience note lands. SinkDisplay marks a note
// that is only ever shown: an unfinished item is a reminder for the person
// reading it, not a reusable experience, so accepting it has nowhere to go.
const (
	SinkMemory  = "memory"
	SinkDisplay = "display"
)

// maxEntries caps one session's output: a close that yields more is padding.
const maxEntries = 8

// entryPayload is one element of the model's answer before validation.
type entryPayload struct {
	Kind     string `json:"kind"`
	Body     string `json:"body"`
	Evidence string `json:"evidence"`
}

// Entry is one candidate distilled from a session. It stays a candidate: nothing
// reaches memory until a person accepts it.
type Entry struct {
	Kind     string `json:"kind"`
	Body     string `json:"body"`
	Evidence string `json:"evidence,omitempty"`
}

// Sink is the destination an accepted entry belongs to. The kind decides it, so
// the model never chooses a destination.
func Sink(kind string) string {
	if kind == KindHandoff {
		return SinkDisplay
	}
	return SinkMemory
}

// parseEntries reads the model's JSON array. An empty array is a valid answer —
// a session with nothing reusable must be stored once rather than retried — so
// only text carrying no array at all is unparseable. A reply the output cap cut
// short still contributes the notes it managed to finish.
func parseEntries(raw string) ([]Entry, bool) {
	body, closed := extractJSONArray(raw)
	if body == "" {
		return nil, false
	}
	var parsed []entryPayload
	if !closed || json.Unmarshal([]byte(body), &parsed) != nil {
		if parsed = salvageEntryObjects(body); len(parsed) == 0 {
			return nil, false
		}
	}
	return normalizeEntries(parsed), true
}

// salvageEntryObjects pulls the complete objects out of an array that was cut
// short, so a truncated answer loses its tail rather than all of its notes.
func salvageEntryObjects(body string) []entryPayload {
	var out []entryPayload
	depth, start, escaped, inString := 0, -1, false, false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth > 0 || start < 0 {
				continue
			}
			var item entryPayload
			if json.Unmarshal([]byte(body[start:i+1]), &item) == nil {
				out = append(out, item)
			}
			start = -1
		}
	}
	return out
}

// normalizeEntries keeps the usable notes in order, dropping the rest.
func normalizeEntries(parsed []entryPayload) []Entry {
	out := make([]Entry, 0, len(parsed))
	seen := map[string]bool{}
	for _, item := range parsed {
		kind := normalizeKind(item.Kind)
		text := strings.TrimSpace(item.Body)
		if kind == "" || text == "" {
			continue
		}
		key := kind + "\x00" + text
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Entry{Kind: kind, Body: text, Evidence: strings.TrimSpace(item.Evidence)})
		if len(out) == maxEntries {
			break
		}
	}
	return out
}

// extractJSONArray returns the array an answer carries, tolerating a fenced or
// prefaced reply. closed reports whether the closing bracket was there: without
// it the reply was cut short and only its complete objects survive.
func extractJSONArray(raw string) (body string, closed bool) {
	start := strings.IndexByte(raw, '[')
	if start < 0 {
		return "", false
	}
	end := strings.LastIndexByte(raw, ']')
	if end < start {
		return raw[start:], false
	}
	return raw[start : end+1], true
}

// normalizeKind maps the labels a model may reach for onto the four kinds.
func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindFact, "facts":
		return KindFact
	case KindRootCause, "rootcause", "root cause", "root_cause":
		return KindRootCause
	case KindRefuted, "refutation", "rejected":
		return KindRefuted
	case KindHandoff, "follow-up", "follow-up ", "followups", "follow-up items", "todo", "todos":
		return KindHandoff
	}
	return ""
}

// encodeEntries renders entries for the projection; an empty list is stored as
// an empty array so a session with nothing reusable is not re-read.
func encodeEntries(entries []Entry) string {
	if len(entries) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// decodeEntries tolerates a projection holding a truncated or older payload.
func decodeEntries(raw string) []Entry {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []Entry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	if len(out) > maxEntries {
		out = out[:maxEntries]
	}
	return out
}
