package agentbus

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus/board"
)

// TalkKind is the closed set of talk records. A close record is how a topic
// becomes read-only: the decision is recorded, never re-derived, so replaying a
// log cannot depend on when it is read (AGENT_BUS §11.1, §11.4).
type TalkKind string

const (
	TalkSay    TalkKind = "say"
	TalkAsk    TalkKind = "ask"
	TalkAnswer TalkKind = "answer"
	TalkClose  TalkKind = "close"
)

// Close reasons, in the order the contract lists them.
const (
	CloseRounds  = "rounds"
	CloseBudget  = "budget"
	CloseSilence = "silence"
	CloseRate    = "rate"
)

// TalkLimits bounds one topic and one question chain. Zero values mean the limit
// is off, which is what a first slice wants: no invented ceilings.
type TalkLimits struct {
	MaxRounds     int
	TokenBudget   int64
	SilenceWindow time.Duration
	RateWindow    time.Duration
	RateMax       int
	MaxHop        int
	AskTTL        time.Duration
}

// TalkLine is one talk record as the log stores it.
type TalkLine struct {
	Seq         uint64           `json:"seq"`
	Topic       string           `json:"topic"`
	Kind        TalkKind         `json:"kind"`
	From        string           `json:"from"`
	To          string           `json:"to,omitempty"`
	Mentions    []string         `json:"mentions,omitempty"`
	Text        string           `json:"text,omitempty"`
	At          time.Time        `json:"at"`
	Correlation string           `json:"correlation,omitempty"`
	Tokens      int64            `json:"tokens,omitempty"`
	Deadline    time.Time        `json:"deadline,omitzero"`
	Reason      string           `json:"reason,omitempty"`
	Evidence    []board.Evidence `json:"evidence,omitempty"`
}

// Topic is one conversation surface as the folded log sees it.
type Topic struct {
	Name        string
	Lines       []TalkLine
	Tokens      int64
	Closed      bool
	CloseReason string
	LastAt      time.Time
}

// Chain tracks one correlation's budget, deadline and hop count.
type Chain struct {
	ID       string
	Hops     int
	Tokens   int64
	OpenedAt time.Time
	Deadline time.Time
}

// TalkState is the folded talk surface: topics plus the question chains they
// carry. Fold it from lines alone; nothing here reads a clock.
type TalkState struct {
	Topics map[string]*Topic
	Chains map[string]*Chain
}

// NewTalkState returns an empty talk surface.
func NewTalkState() *TalkState {
	return &TalkState{Topics: map[string]*Topic{}, Chains: map[string]*Chain{}}
}

// FoldTalk replays lines in log order. Like the board's fold it is a pure
// function of the records, so the same lines always give the same topics.
func FoldTalk(lines []TalkLine) *TalkState {
	st := NewTalkState()
	for _, line := range lines {
		if err := ApplyTalk(st, line, TalkLimits{}); err != nil {
			continue
		}
	}
	return st
}

// TalkReject explains a refused talk line.
type TalkReject struct {
	Topic  string
	Kind   TalkKind
	Reason string
}

func (e *TalkReject) Error() string {
	return fmt.Sprintf("agentbus talk: %s refused on %q: %s", e.Kind, e.Topic, e.Reason)
}

// Refusal reasons. Callers and tests match on these strings.
const (
	RefuseNoTopic       = "no_topic"
	RefuseTopicClosed   = "topic_closed"
	RefuseRounds        = "rounds_exhausted"
	RefuseBudget        = "budget_exhausted"
	RefuseRate          = "rate_limited"
	RefuseExpired       = "expired"
	RefuseHop           = "hop_exhausted"
	RefuseNoText        = "no_text"
	RefuseNoCorrelation = "no_correlation"
	RefuseUnknownKind   = "unknown_kind"
)

func refuseTalk(topic string, kind TalkKind, reason string) error {
	return &TalkReject{Topic: topic, Kind: kind, Reason: reason}
}

// IsTalkReject reports whether err is a talk refusal and returns its reason.
func IsTalkReject(err error) (string, bool) {
	var rej *TalkReject
	if errors.As(err, &rej) {
		return rej.Reason, true
	}
	return "", false
}

// ApplyTalk validates one record against the folded surface and mutates it.
// Acceptance depends only on recorded state: silence lapses are recorded
// separately by CloseLine, so a replay is never time-dependent.
func ApplyTalk(st *TalkState, line TalkLine, lim TalkLimits) error {
	topic := strings.TrimSpace(line.Topic)
	if topic == "" {
		return refuseTalk(line.Topic, line.Kind, RefuseNoTopic)
	}
	t := st.Topics[topic]
	if t == nil {
		t = &Topic{Name: topic}
		st.Topics[topic] = t
	}
	switch line.Kind {
	case TalkClose:
		return applyTalkClose(t, line)
	case TalkSay, TalkAsk, TalkAnswer:
	default:
		return refuseTalk(topic, line.Kind, RefuseUnknownKind)
	}
	if t.Closed {
		return refuseTalk(topic, line.Kind, RefuseTopicClosed)
	}
	if strings.TrimSpace(line.Text) == "" {
		return refuseTalk(topic, line.Kind, RefuseNoText)
	}
	if lim.MaxRounds > 0 && len(t.Lines) >= lim.MaxRounds {
		return refuseTalk(topic, line.Kind, RefuseRounds)
	}
	if lim.TokenBudget > 0 && t.Tokens+line.Tokens > lim.TokenBudget {
		return refuseTalk(topic, line.Kind, RefuseBudget)
	}
	if rateLimited(t, line, lim) {
		return refuseTalk(topic, line.Kind, RefuseRate)
	}
	if line.Kind == TalkAsk || line.Kind == TalkAnswer {
		if err := applyTalkChain(st, line, lim); err != nil {
			return err
		}
	}
	t.Lines = append(t.Lines, line)
	t.Tokens += line.Tokens
	t.LastAt = line.At
	return nil
}

func applyTalkClose(t *Topic, line TalkLine) error {
	if t.Closed {
		return refuseTalk(t.Name, line.Kind, RefuseTopicClosed)
	}
	t.Closed = true
	t.CloseReason = strings.TrimSpace(line.Reason)
	if t.CloseReason == "" {
		t.CloseReason = CloseSilence
	}
	t.Lines = append(t.Lines, line)
	t.LastAt = line.At
	return nil
}

// applyTalkChain meters one correlation: opening asks set the deadline, every
// question and answer spends a hop, and an expired chain refuses both.
func applyTalkChain(st *TalkState, line TalkLine, lim TalkLimits) error {
	id := strings.TrimSpace(line.Correlation)
	if id == "" {
		return refuseTalk(strings.TrimSpace(line.Topic), line.Kind, RefuseNoCorrelation)
	}
	chain := st.Chains[id]
	if chain == nil {
		if line.Kind == TalkAnswer {
			return refuseTalk(line.Topic, line.Kind, RefuseNoCorrelation)
		}
		chain = &Chain{ID: id, OpenedAt: line.At}
		if lim.AskTTL > 0 {
			chain.Deadline = line.At.Add(lim.AskTTL)
		}
		st.Chains[id] = chain
	} else if !chain.Deadline.IsZero() && line.At.After(chain.Deadline) {
		return refuseTalk(line.Topic, line.Kind, RefuseExpired)
	}
	if lim.MaxHop > 0 && chain.Hops >= lim.MaxHop {
		return refuseTalk(line.Topic, line.Kind, RefuseHop)
	}
	if lim.TokenBudget > 0 && chain.Tokens+line.Tokens > lim.TokenBudget {
		return refuseTalk(line.Topic, line.Kind, RefuseBudget)
	}
	chain.Hops++
	chain.Tokens += line.Tokens
	return nil
}

func rateLimited(t *Topic, line TalkLine, lim TalkLimits) bool {
	if lim.RateWindow <= 0 || lim.RateMax <= 0 || line.At.IsZero() {
		return false
	}
	cutoff := line.At.Add(-lim.RateWindow)
	count := 0
	for _, prev := range t.Lines {
		if prev.Kind == TalkClose || prev.At.Before(cutoff) {
			continue
		}
		count++
	}
	return count >= lim.RateMax
}

// TopicLapsed reports the derived silence predicate: nobody has spoken for
// longer than the window. It is a predicate, not a transition.
func TopicLapsed(st *TalkState, topic string, now time.Time, lim TalkLimits) bool {
	if lim.SilenceWindow <= 0 {
		return false
	}
	t := st.Topics[topic]
	if t == nil || t.Closed || t.LastAt.IsZero() {
		return false
	}
	return now.Sub(t.LastAt) > lim.SilenceWindow
}

// CloseLine builds the record that closes a lapsed topic. Whoever writes next
// appends it, the same way an expired board lease is reclaimed by its next
// writer; the idempotency key is the topic plus the line it lapsed from.
func CloseLine(st *TalkState, topic string, now time.Time) (TalkLine, bool) {
	t := st.Topics[topic]
	if t == nil || t.Closed || t.LastAt.IsZero() {
		return TalkLine{}, false
	}
	return TalkLine{
		Topic:  topic,
		Kind:   TalkClose,
		From:   "system",
		At:     now,
		Reason: CloseSilence,
	}, true
}

// Digest returns what one participant is allowed to see from a topic: the lines
// that name them, and a count of the rest. Speech that names nobody stays out of
// everybody's context — that rule is what keeps free talk from becoming a
// cost bomb (AGENT_BUS §3.2).
func Digest(st *TalkState, topic, participant string, cursor uint64) (named []TalkLine, hidden int) {
	t := st.Topics[topic]
	if t == nil {
		return nil, 0
	}
	for _, line := range t.Lines {
		if line.Seq <= cursor || line.Kind == TalkClose {
			continue
		}
		if addresses(line, participant) {
			named = append(named, line)
			continue
		}
		hidden++
	}
	return named, hidden
}

func addresses(line TalkLine, participant string) bool {
	if participant == "" {
		return false
	}
	if line.From == participant || line.To == participant {
		return true
	}
	return slices.Contains(line.Mentions, participant)
}

// TopicNames lists the topics a reader can enumerate, sorted so two reads agree.
func (st *TalkState) TopicNames() []string {
	out := make([]string, 0, len(st.Topics))
	for name := range st.Topics {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
