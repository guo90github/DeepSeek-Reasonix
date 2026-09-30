package recap

import (
	"encoding/json"
	"strings"
	"unicode"
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

// The three deposition tiers a note can be proposed for. The model proposes, the
// rule decides: a note about this repository belongs to the project, a note about
// this machine or about work in general belongs to the person, and a note only one
// project ever saw must not claim otherwise.
const (
	ScopeProject = "project"
	ScopeBase    = "base"
	ScopeGeneric = "generic"
)

// Ref kinds: where the conclusion can be checked. A note that says "the count was
// wrong" is worth less than one that says which line, which command, or which
// turns to read.
const (
	RefPath    = "path"
	RefCommand = "command"
	RefTest    = "test"
	RefTurn    = "turn"
	RefConfig  = "config"
)

// maxRefs caps what one note cites: a pointer list longer than a line is not a
// pointer, it is a second body.
const maxRefs = 4

// Ref is one place a note can be checked at: a file, a command, a test, a turn
// range, or a config key.
type Ref struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

// Scope is the deposition tier a note was proposed for. Reason is kept so a
// person can see why the model thought a note was universal before accepting it.
type Scope struct {
	Level  string `json:"level,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// entryPayload is one element of the model's answer before validation.
type entryPayload struct {
	Kind     string        `json:"kind"`
	Body     string        `json:"body"`
	Evidence string        `json:"evidence"`
	Refs     []Ref         `json:"refs"`
	Scope    *scopePayload `json:"scope"`
}

type scopePayload struct {
	Level  string `json:"level"`
	Reason string `json:"reason"`
}

// Entry is one candidate distilled from a session. It stays a candidate: nothing
// reaches memory until a person accepts it.
type Entry struct {
	Kind     string `json:"kind"`
	Body     string `json:"body"`
	Evidence string `json:"evidence,omitempty"`
	Refs     []Ref  `json:"refs,omitempty"`
	Scope    Scope  `json:"scope,omitempty"`
}

// MemoryScopeFor maps a proposed level onto the memory scope a note would land in
// once a person lets the tier decide it. Nothing calls it yet on purpose: memory is
// written only on an explicit accept, and every accept still lands in the project.
// The rule is here — and tested — so a tier's meaning is pinned before anyone opts
// into letting it choose.
func MemoryScopeFor(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case ScopeBase, ScopeGeneric:
		return "global"
	}
	return "project"
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
		out = append(out, Entry{
			Kind:     kind,
			Body:     text,
			Evidence: strings.TrimSpace(item.Evidence),
			Refs:     normalizeRefs(item.Refs),
			Scope:    normalizeScope(item.Scope),
		})
		if len(out) == maxEntries {
			break
		}
	}
	return mergeSameTopic(out)
}

// mergeSameTopic keeps one entry per topic: a long session restates the same
// decision over several turns, and a button group per restatement is a cost the
// reader pays for nothing. Kinds never merge across each other, and the surviving
// entry absorbs every pointer and evidence line the restatements carried.
func mergeSameTopic(entries []Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		index := sameTopicIndex(out, entry)
		if index < 0 {
			out = append(out, entry)
			continue
		}
		out[index].Evidence = joinEvidence(out[index].Evidence, entry.Evidence)
		for _, ref := range entry.Refs {
			out[index].Refs = appendUniqueRefs(out[index].Refs, ref)
		}
	}
	return out
}

func appendUniqueRefs(refs []Ref, ref Ref) []Ref {
	for _, existing := range refs {
		if existing == ref {
			return refs
		}
	}
	return append(refs, ref)
}

func sameTopicIndex(entries []Entry, entry Entry) int {
	tokens := topicTokens(entry.Body)
	for i := range entries {
		if entries[i].Kind != entry.Kind {
			continue
		}
		if sameTopicTokens(tokens, topicTokens(entries[i].Body)) {
			return i
		}
	}
	return -1
}

// sameTopicTokens is deliberately narrow — shared identifiers, or heavy overlap —
// because a wrong merge hides a note the next session never gets to read.
func sameTopicTokens(a, b map[string]bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	shared, sharedIdentifiers := 0, 0
	for token := range a {
		if !b[token] {
			continue
		}
		shared++
		if !isCJKToken(token) {
			sharedIdentifiers++
		}
	}
	return shared >= 2 && (sharedIdentifiers >= 1 || shared >= 4)
}

// topicTokens keeps what carries a topic: identifiers as whole words, CJK as
// adjacent pairs (a Chinese sentence reworded shares pairs, not words).
func topicTokens(s string) map[string]bool {
	tokens := map[string]bool{}
	var ascii, cjk []rune
	flushASCII := func() {
		if len(ascii) >= 2 {
			tokens[string(ascii)] = true
		}
		ascii = ascii[:0]
	}
	flushCJK := func() {
		for i := 0; i+1 < len(cjk); i++ {
			tokens[string(cjk[i:i+2])] = true
		}
		cjk = cjk[:0]
	}
	for _, r := range []rune(strings.ToLower(s)) {
		switch {
		case isCJK(r):
			flushASCII()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			flushCJK()
			ascii = append(ascii, r)
		default:
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r)
}

func isCJKToken(token string) bool {
	for _, r := range token {
		return isCJK(r)
	}
	return false
}

func joinEvidence(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "" || strings.Contains(a, b):
		return a
	case strings.Contains(b, a):
		return b
	}
	return a + " " + b
}

// normalizeRefs keeps the usable pointers. A missing or unrecognized kind is
// inferred from the value rather than dropped: the pointer is the part worth
// having, and a model that only fills values still produced them.
func normalizeRefs(parsed []Ref) []Ref {
	out := make([]Ref, 0, len(parsed))
	for _, ref := range parsed {
		value := strings.TrimSpace(ref.Value)
		if value == "" {
			continue
		}
		out = append(out, Ref{
			Kind:   normalizeRefKind(ref.Kind, value),
			Value:  value,
			Detail: strings.TrimSpace(ref.Detail),
		})
		if len(out) == maxRefs {
			break
		}
	}
	return out
}

// normalizeRefKind maps a label onto one of the five kinds, inferring from the
// value when the label is missing or unexpected.
func normalizeRefKind(kind, value string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case RefPath, "file", "files", "source":
		return RefPath
	case RefCommand, "cmd", "shell", "bash":
		return RefCommand
	case RefTest, "tests", "case":
		return RefTest
	case RefTurn, "turns", "turn-range", "range":
		return RefTurn
	case RefConfig, "setting", "key", "toml":
		return RefConfig
	}
	switch {
	case strings.ContainsAny(value, `/\`) && !strings.Contains(value, " "):
		return RefPath
	case onlyDigitsAndDashes(value):
		return RefTurn
	case strings.Contains(value, " "):
		return RefCommand
	}
	return RefPath
}

// normalizeScope keeps a proposed tier only when it is one of the three; anything
// else leaves the note where every unproposed note belongs, in its own project.
func normalizeScope(parsed *scopePayload) Scope {
	if parsed == nil {
		return Scope{}
	}
	level := strings.ToLower(strings.TrimSpace(parsed.Level))
	switch level {
	case ScopeProject, ScopeBase, ScopeGeneric:
		return Scope{Level: level, Reason: strings.TrimSpace(parsed.Reason)}
	}
	return Scope{}
}

func onlyDigitsAndDashes(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
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
