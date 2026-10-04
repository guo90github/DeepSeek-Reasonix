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

// HearingKind is the closed set of deliberation records. A hearing is opened on a
// contested node, the participants who must answer are named, and it ends with a
// verdict that either side may act on (AGENT_BUS §3.2).
type HearingKind string

const (
	HearingOpen     HearingKind = "open"
	HearingAnswer   HearingKind = "answer"
	HearingEscalate HearingKind = "escalate"
	HearingRule     HearingKind = "rule"
	HearingClose    HearingKind = "close"
)

// Verdicts. `undecided-by-rule` is a real outcome, not a failure: it records that
// the question exhausted its escalation quota and nobody may pretend it was
// settled.
const (
	VerdictStands     = "stands"
	VerdictRefuted    = "refuted"
	VerdictEscalate   = "escalate"
	VerdictUndecided  = "undecided-by-rule"
	ReasonWeight      = "evidence-weight"
	ReasonEqualWeight = "equal-weight"
	ReasonQuota       = "escalation-quota"
)

// Hearing refusal reasons.
const (
	RefuseHearingNoNode      = "hearing_node_missing"
	RefuseHearingNoRequired  = "hearing_no_required"
	RefuseHearingAlreadyOpen = "hearing_already_open"
	RefuseHearingNotOpen     = "hearing_not_open"
	RefuseHearingClosed      = "hearing_closed"
	RefuseHearingCooldown    = "hearing_cooldown"
	RefuseHearingNoActor     = "hearing_no_actor"
	RefuseHearingAnswered    = "hearing_already_answered"
	RefuseHearingRounds      = "hearing_rounds_exhausted"
	RefuseHearingKind        = "hearing_unknown_kind"
)

// HearingLimits bounds a deliberation. Zero values leave a bound off.
type HearingLimits struct {
	// MaxRounds is how many times each required participant may be asked.
	MaxRounds int
	// RoundTTL is how long one round waits before silence counts as no_answer.
	RoundTTL time.Duration
	// Cooldown keeps a closed question from being re-opened immediately.
	Cooldown time.Duration
	// EscalationQuota is how many equal-weight questions may go to a human before
	// they start closing as undecided-by-rule.
	EscalationQuota int
}

// HearingRecord is one deliberation record as the log stores it.
type HearingRecord struct {
	Seq      uint64           `json:"seq"`
	Node     string           `json:"node"`
	Kind     HearingKind      `json:"kind"`
	Actor    string           `json:"actor,omitempty"`
	Text     string           `json:"text,omitempty"`
	Required []string         `json:"required,omitempty"`
	Evidence []board.Evidence `json:"evidence,omitempty"`
	Verdict  string           `json:"verdict,omitempty"`
	Outcome  string           `json:"outcome,omitempty"`
	Reason   string           `json:"reason,omitempty"`
	At       time.Time        `json:"at"`
}

// Hearing is the deliberation over one node as the folded records see it. Rounds
// and answers are derived from the records and their own timestamps, so a replay
// never depends on when it is read.
type Hearing struct {
	Node        string
	Records     []HearingRecord
	Required    []string
	Open        bool
	Verdict     string
	Outcome     string
	Reason      string
	Escalated   bool
	EscalatedTo string
	OpenedAt    time.Time
	ClosedAt    time.Time
	LastAt      time.Time
}

// HearingState is the folded deliberation surface of one board plus the
// board-level escalation ledger.
type HearingState struct {
	Hearings    map[string]*Hearing
	Escalations int
}

// NewHearingState returns an empty deliberation surface.
func NewHearingState() *HearingState {
	return &HearingState{Hearings: map[string]*Hearing{}}
}

// FoldHearings replays records in log order. Acceptance uses no limits, so the
// fold reflects what the log says rather than what a reader would allow.
func FoldHearings(records []HearingRecord) *HearingState {
	st := NewHearingState()
	for _, rec := range records {
		if err := ApplyHearing(st, rec, HearingLimits{}); err != nil {
			continue
		}
	}
	return st
}

// HearingReject explains a refused deliberation record.
type HearingReject struct {
	Node   string
	Kind   HearingKind
	Reason string
}

func (e *HearingReject) Error() string {
	return fmt.Sprintf("agentbus hearing: %s refused on %q: %s", e.Kind, e.Node, e.Reason)
}

// IsHearingReject reports whether err is a hearing refusal and returns its reason.
func IsHearingReject(err error) (string, bool) {
	var rej *HearingReject
	if errors.As(err, &rej) {
		return rej.Reason, true
	}
	return "", false
}

func refuseHearing(node string, kind HearingKind, reason string) error {
	return &HearingReject{Node: node, Kind: kind, Reason: reason}
}

// ApplyHearing validates one record and mutates the surface. Silence, deadlines
// and rounds are all decided from the records' own timestamps: a reader at any
// later moment folds the same deliberation.
func ApplyHearing(st *HearingState, rec HearingRecord, lim HearingLimits) error {
	node := strings.TrimSpace(rec.Node)
	if node == "" {
		return refuseHearing(rec.Node, rec.Kind, RefuseHearingNoNode)
	}
	h := st.Hearings[node]
	switch rec.Kind {
	case HearingOpen:
		return applyHearingOpen(st, h, node, rec, lim)
	case HearingAnswer:
		return applyHearingAnswer(h, node, rec, lim)
	case HearingEscalate, HearingRule, HearingClose:
		return applyHearingClose(st, h, node, rec)
	default:
		return refuseHearing(node, rec.Kind, RefuseHearingKind)
	}
}

func applyHearingOpen(st *HearingState, h *Hearing, node string, rec HearingRecord, lim HearingLimits) error {
	if h != nil && h.Open {
		return refuseHearing(node, rec.Kind, RefuseHearingAlreadyOpen)
	}
	required := dedupSorted(rec.Required)
	if len(required) == 0 {
		return refuseHearing(node, rec.Kind, RefuseHearingNoRequired)
	}
	// The cooldown compares the log's own timestamps, so a replay reaches the same
	// verdict anywhere without consulting a wall clock.
	if h != nil && lim.Cooldown > 0 && !h.ClosedAt.IsZero() && h.ClosedAt.Add(lim.Cooldown).After(rec.At) {
		return refuseHearing(node, rec.Kind, RefuseHearingCooldown)
	}
	if h == nil {
		h = &Hearing{Node: node}
		st.Hearings[node] = h
	}
	h.Open = true
	h.Required = required
	h.Verdict, h.Outcome, h.Reason, h.Escalated, h.EscalatedTo = "", "", "", false, ""
	h.OpenedAt = rec.At
	h.ClosedAt = time.Time{}
	h.LastAt = rec.At
	h.Records = append(h.Records, rec)
	return nil
}

func applyHearingAnswer(h *Hearing, node string, rec HearingRecord, lim HearingLimits) error {
	if h == nil || !h.Open {
		return refuseHearing(node, rec.Kind, RefuseHearingNotOpen)
	}
	if strings.TrimSpace(rec.Actor) == "" {
		return refuseHearing(node, rec.Kind, RefuseHearingNoActor)
	}
	if lim.MaxRounds > 0 && h.Round() > lim.MaxRounds {
		return refuseHearing(node, rec.Kind, RefuseHearingRounds)
	}
	if containsString(h.AnsweredThisRound(), rec.Actor) {
		return refuseHearing(node, rec.Kind, RefuseHearingAnswered)
	}
	h.Records = append(h.Records, rec)
	h.LastAt = rec.At
	return nil
}

func applyHearingClose(st *HearingState, h *Hearing, node string, rec HearingRecord) error {
	if h == nil || (!h.Open && rec.Kind != HearingClose) {
		return refuseHearing(node, rec.Kind, RefuseHearingNotOpen)
	}
	if !h.Open {
		return refuseHearing(node, rec.Kind, RefuseHearingClosed)
	}
	switch rec.Verdict {
	case VerdictStands, VerdictRefuted, VerdictEscalate, VerdictUndecided:
	default:
		return refuseHearing(node, rec.Kind, RefuseHearingKind)
	}
	h.Open = false
	h.Verdict = rec.Verdict
	h.Outcome = rec.Outcome
	h.Reason = rec.Reason
	h.ClosedAt = rec.At
	h.LastAt = rec.At
	if rec.Verdict == VerdictEscalate {
		h.Escalated = true
		h.EscalatedTo = strings.TrimSpace(rec.Actor)
		st.Escalations++
	}
	h.Records = append(h.Records, rec)
	return nil
}

// hearingDetail names a deliberation in one line, including the rule that closed it: the
// reason is what tells a quota escalation from an evidence-weight verdict, and it otherwise
// lives only in hearings.jsonl (2026-10-05).
func hearingDetail(detail string, h *Hearing) string {
	if h == nil {
		return detail
	}
	if reason := strings.TrimSpace(h.Reason); reason != "" {
		return detail + " (" + reason + ")"
	}
	return detail
}

// Round is the current asking round, derived: every required participant
// answering once completes a round.
func (h *Hearing) Round() int {
	if len(h.Required) == 0 {
		return 1
	}
	answered := map[string]bool{}
	passes := 0
	for _, rec := range h.Records {
		if rec.Kind != HearingAnswer || !containsString(h.Required, rec.Actor) {
			continue
		}
		answered[rec.Actor] = true
		if len(answered) >= len(h.Required) {
			passes++
			answered = map[string]bool{}
		}
	}
	return passes + 1
}

// AnsweredThisRound lists who has answered since the current round began.
func (h *Hearing) AnsweredThisRound() []string {
	if len(h.Required) == 0 {
		return nil
	}
	answered := map[string]bool{}
	round := []string{}
	for _, rec := range h.Records {
		if rec.Kind != HearingAnswer || !containsString(h.Required, rec.Actor) {
			continue
		}
		answered[rec.Actor] = true
		round = append(round, rec.Actor)
		if len(answered) >= len(h.Required) {
			round = nil
			answered = map[string]bool{}
		}
	}
	return dedupSorted(round)
}

// RoundStart is when the current round began, taken from the records themselves:
// the last answer, or the opening when nobody has answered yet.
func (h *Hearing) RoundStart() time.Time {
	start := h.OpenedAt
	for _, rec := range h.Records {
		if rec.Kind == HearingAnswer && rec.At.After(start) {
			start = rec.At
		}
	}
	return start
}

// HearingSilent lists required participants who have not answered the current
// round although the round's window has passed. Silence is visible work, never a
// quiet deletion (T6-4).
func HearingSilent(h *Hearing, now time.Time, lim HearingLimits) []string {
	if h == nil || !h.Open || lim.RoundTTL <= 0 {
		return nil
	}
	if now.Sub(h.RoundStart()) <= lim.RoundTTL {
		return nil
	}
	answered := h.AnsweredThisRound()
	out := make([]string, 0, len(h.Required))
	for _, participant := range h.Required {
		if !containsString(answered, participant) {
			out = append(out, participant)
		}
	}
	return out
}

// HearingRequired names who must answer about a node: whoever owns it now and
// everyone who refuted it.
func HearingRequired(st *board.State, node string) []string {
	if st == nil {
		return nil
	}
	n, ok := st.Nodes[node]
	if !ok {
		return nil
	}
	out := []string{}
	if n.Owner != "" {
		out = append(out, n.Owner)
	}
	for _, refute := range n.Refutes {
		out = append(out, refute.Actor)
	}
	return dedupSorted(out)
}

// VerifiableWeight counts evidence someone can go and check, counting each reference
// once: a bare claim weighs nothing, and citing the same test twice is still one test.
// Both halves matter — without the first, opinions outvote a test; without the second,
// repetition does (T6-6, T5-6).
func VerifiableWeight(evidence []board.Evidence) int {
	seen := map[string]bool{}
	for _, item := range evidence {
		ref := strings.TrimSpace(item.Ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
	}
	return len(seen)
}

// WeighResponse decides a contested node by evidence weight, never by how many
// said it. Equal weight goes to a human until the escalation quota runs out, and
// then closes as undecided-by-rule — recorded, so nobody can present it as
// settled.
func WeighResponse(claimSupport, refuteSupport []board.Evidence, escalated, quota int) (verdict, reason string) {
	claimWeight := VerifiableWeight(claimSupport)
	refuteWeight := VerifiableWeight(refuteSupport)
	switch {
	case claimWeight > refuteWeight:
		return VerdictStands, ReasonWeight
	case refuteWeight > claimWeight:
		return VerdictRefuted, ReasonWeight
	}
	if escalated < quota {
		return VerdictEscalate, ReasonEqualWeight
	}
	return VerdictUndecided, ReasonQuota
}

func dedupSorted(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || containsString(out, item) {
			continue
		}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func containsString(items []string, item string) bool {
	return slices.Contains(items, item)
}
