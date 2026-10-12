package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"reasonix/internal/event"
)

type acceptedDecision struct {
	ID       string
	Question string
	Answer   string
	// Auto marks a decision the host made for the user after the no-answer
	// timeout. Reuse must report it as a host fallback, not as a user choice.
	Auto      bool
	Ambiguity decisionAmbiguity
}

type decisionAmbiguity struct {
	Headers map[string]struct{}
	Options map[string]struct{}
	Terms   map[string]struct{}
}

// decisionMemory is the conversation's record of the decisions already settled,
// by the user or by the host's no-answer fallback. It outlives one Agent.Run, so
// a later turn reuses the settled answer instead of putting the same question to
// the user a second time.
type decisionMemory struct {
	mu        sync.Mutex
	decisions map[string]acceptedDecision
}

func newDecisionMemory() *decisionMemory {
	return &decisionMemory{decisions: map[string]acceptedDecision{}}
}

// Every method tolerates a nil receiver: a tool call outside a wired agent
// context simply carries no decision memory.
func (m *decisionMemory) remember(d acceptedDecision) {
	if m == nil || d.ID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.decisions == nil {
		m.decisions = map[string]acceptedDecision{}
	}
	m.decisions[d.ID] = d
}

func (m *decisionMemory) get(id string) (acceptedDecision, bool) {
	if m == nil || id == "" {
		return acceptedDecision{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.decisions[id]
	return d, ok
}

func (m *decisionMemory) snapshot() []acceptedDecision {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]acceptedDecision, 0, len(m.decisions))
	for _, decision := range m.decisions {
		out = append(out, decision)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type decisionMemoryContextKey struct{}

func withDecisionMemory(ctx context.Context, memory *decisionMemory) context.Context {
	if memory == nil {
		return ctx
	}
	return context.WithValue(ctx, decisionMemoryContextKey{}, memory)
}

func decisionMemoryFrom(ctx context.Context) *decisionMemory {
	memory, _ := ctx.Value(decisionMemoryContextKey{}).(*decisionMemory)
	return memory
}

// decisionMemory returns the conversation's accepted-decision memory, created on
// first use. A tool call carries it through withAgentContext.
func (r *sessionRuntime) decisionMemory() *decisionMemory {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.decisions == nil {
		r.decisions = newDecisionMemory()
	}
	return r.decisions
}

func decisionIDForQuestions(qs []event.AskQuestion) string {
	h := sha256.New()
	for _, q := range qs {
		_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(q.Prompt))))
		_, _ = h.Write([]byte{0})
		for _, opt := range q.Options {
			_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(opt.Label))))
			_, _ = h.Write([]byte{0})
		}
	}
	return "dec-" + hex.EncodeToString(h.Sum(nil)[:8])
}

type turnStateContextKey struct{}

func withTurnState(ctx context.Context, turn *turnRuntime) context.Context {
	if turn == nil {
		return ctx
	}
	return context.WithValue(ctx, turnStateContextKey{}, turn)
}

func turnStateFrom(ctx context.Context) *turnRuntime {
	turn, _ := ctx.Value(turnStateContextKey{}).(*turnRuntime)
	return turn
}

func rememberDecisionForQuestions(ctx context.Context, id, question, answer string, auto bool, qs []event.AskQuestion) {
	decisionMemoryFrom(ctx).remember(acceptedDecision{
		ID:        id,
		Question:  question,
		Answer:    answer,
		Auto:      auto,
		Ambiguity: decisionAmbiguityForQuestions(qs),
	})
}

func existingDecision(ctx context.Context, id string) (acceptedDecision, bool) {
	return decisionMemoryFrom(ctx).get(id)
}

func matchingExistingDecision(ctx context.Context, qs []event.AskQuestion) (acceptedDecision, bool) {
	candidate := decisionAmbiguityForQuestions(qs)
	for _, decision := range decisionMemoryFrom(ctx).snapshot() {
		if sameDecisionAmbiguity(decision.Ambiguity, candidate) {
			return decision, true
		}
	}
	return acceptedDecision{}, false
}

// decisionReuseText states what reuse is reusing. An auto-selected answer is the
// host's fallback; a model reading it as the user's own decision would treat a
// guess as settled intent.
func decisionReuseText(dec acceptedDecision) string {
	if dec.Auto {
		return "The host auto-selected " + dec.Answer + " after no answer; the user did not choose it."
	}
	return "The user already chose: " + dec.Answer + "."
}

// knownDecisionList names the decisions a rejected decision_id could have meant.
// One failed call then teaches the model the id it needs, instead of it guessing
// again — a guess that costs the user another prompt.
func knownDecisionList(ctx context.Context, limit int) string {
	decisions := decisionMemoryFrom(ctx).snapshot()
	if len(decisions) == 0 || limit <= 0 {
		return ""
	}
	parts := make([]string, 0, limit)
	for _, decision := range decisions {
		if len(parts) == limit {
			break
		}
		label := decision.Question
		if label == "" {
			label = decision.ID
		}
		parts = append(parts, decision.ID+" ("+clipDecisionLabel(label, 60)+")")
	}
	return strings.Join(parts, ", ")
}

func clipDecisionLabel(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func decisionAmbiguityForQuestions(qs []event.AskQuestion) decisionAmbiguity {
	out := decisionAmbiguity{
		Headers: map[string]struct{}{},
		Options: map[string]struct{}{},
		Terms:   map[string]struct{}{},
	}
	for _, q := range qs {
		addDecisionPhrase(out.Headers, q.Header)
		addDecisionTerms(out.Terms, q.Header)
		addDecisionTerms(out.Terms, q.Prompt)
		for _, option := range q.Options {
			addDecisionPhrase(out.Options, option.Label)
			addDecisionTerms(out.Terms, option.Label)
		}
	}
	return out
}

func addDecisionPhrase(dst map[string]struct{}, value string) {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		dst[b.String()] = struct{}{}
	}
}

func addDecisionTerms(dst map[string]struct{}, value string) {
	var latin strings.Builder
	flush := func() {
		if latin.Len() > 1 {
			dst[latin.String()] = struct{}{}
		}
		latin.Reset()
	}
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			flush()
			dst[string(r)] = struct{}{}
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			latin.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
}

func sameDecisionAmbiguity(left, right decisionAmbiguity) bool {
	if len(left.Terms) == 0 || len(right.Terms) == 0 {
		return false
	}
	headerMatch := setIntersection(left.Headers, right.Headers) > 0
	optionSimilarity := setJaccard(left.Options, right.Options)
	termSimilarity := setJaccard(left.Terms, right.Terms)
	return (headerMatch && (optionSimilarity >= 0.5 || termSimilarity >= 0.45)) ||
		(optionSimilarity >= 0.75 && termSimilarity >= 0.35) || termSimilarity >= 0.7
}

func setIntersection(left, right map[string]struct{}) int {
	count := 0
	for value := range left {
		if _, ok := right[value]; ok {
			count++
		}
	}
	return count
}

func setJaccard(left, right map[string]struct{}) float64 {
	union := len(left) + len(right)
	if union == 0 {
		return 0
	}
	intersection := setIntersection(left, right)
	return float64(intersection) / float64(union-intersection)
}

type askArgs struct {
	DecisionID string `json:"decision_id"`
	Evidence   string `json:"new_evidence"`
	Questions  []struct {
		Header      string `json:"header"`
		Question    string `json:"question"`
		MultiSelect bool   `json:"multiSelect"`
		Options     []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"questions"`
}

func parseAskArgs(raw json.RawMessage) (askArgs, error) {
	var p askArgs
	if err := json.Unmarshal(raw, &p); err != nil {
		return askArgs{}, fmt.Errorf("invalid args: %w", err)
	}
	if len(p.Questions) == 0 {
		return askArgs{}, fmt.Errorf("at least one question is required")
	}
	if len(p.Questions) > 3 {
		return askArgs{}, fmt.Errorf("at most 3 questions may be asked in one clarification")
	}
	return p, nil
}
