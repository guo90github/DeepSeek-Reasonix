package control

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"reasonix/internal/billing"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

const (
	// One audit call covers at most this many turns; the per-turn character
	// allowance is derived from the group size so a call stays inside
	// sessionAuditCallInputChars.
	sessionAuditSegmentMaxTurns = 8
	sessionAuditCallInputChars  = 48000
	sessionAuditMinTurnChars    = 1500
	// sessionAuditMaxSegments bounds the whole run's call count: one call per
	// segment plus one cross-turn review call.
	sessionAuditMaxSegments       = 8
	sessionAuditSegmentMaxTokens  = 4000
	sessionAuditReviewMaxTokens   = 1600
	sessionAuditSegmentTimeout    = 180 * time.Second
	sessionAuditReviewTimeout     = 120 * time.Second
	sessionAuditTurnPromptMaxChar = 2000
	sessionAuditMaxIssues         = 5
)

// SessionAuditTurn is one loaded turn handed to the session audit: its
// session-absolute turn number, the request it answered, and its reasoning
// chain. The request only anchors omission/drift judgements and is truncated
// hard; the reasoning keeps the configured per-turn ceiling.
type SessionAuditTurn struct {
	Turn      int    `json:"turn"`
	Prompt    string `json:"prompt"`
	Reasoning string `json:"reasoning"`
}

// SessionAuditTurnResult is one turn's verdict inside a session audit.
type SessionAuditTurnResult struct {
	Turn             int                  `json:"turn"`
	Score            float64              `json:"score"`
	Contradiction    int                  `json:"contradiction"`
	FactualError     int                  `json:"factualError"`
	InvalidInference int                  `json:"invalidInference"`
	Redundancy       int                  `json:"redundancy"`
	InstructionDrift int                  `json:"instructionDrift"`
	Omission         int                  `json:"omission"`
	Issues           int                  `json:"issues"`
	Conclusion       string               `json:"conclusion"`
	PriorConflict    string               `json:"priorConflict"`
	Explanation      string               `json:"explanation"`
	Findings         []event.AuditFinding `json:"findings"`
	Truncated        bool                 `json:"truncated"`
}

// SessionAuditIssue is one cross-turn problem found by the review pass.
type SessionAuditIssue struct {
	Type  string `json:"type"` // cross_turn_contradiction | cross_turn_drift | repeated_dead_end | unmet_commitment | error_propagation
	Turns []int  `json:"turns"`
	Note  string `json:"note"`
	Quote string `json:"quote"`
}

// SessionAuditTotals is the whole-session verdict, plus the per-turn rows it was
// derived from. It is one-shot UI evidence and is never persisted or fed back
// into the session history.
type SessionAuditTotals struct {
	Audited      bool                     `json:"audited"`
	ElapsedMs    int64                    `json:"elapsedMs"`
	Score        float64                  `json:"score"`
	Trend        string                   `json:"trend"`
	Explanation  string                   `json:"explanation"`
	Issues       []SessionAuditIssue      `json:"issues"`
	Turns        []SessionAuditTurnResult `json:"turns"`
	TurnCount    int                      `json:"turnCount"`
	SegmentCount int                      `json:"segmentCount"`
	EvalTokens   int                      `json:"evalTokens"`
	EvalCost     float64                  `json:"evalCost"`
}

// SessionAuditEvent is one step of a session audit run. Kind is "request"
// (before a model call, carrying the exact prompt and input), "reasoning" /
// "text" (streamed deltas), or "step_done" / "step_failed".
type SessionAuditEvent struct {
	Stage        string `json:"stage"` // segment | review
	Index        int    `json:"index"` // 1-based step number
	Total        int    `json:"total"` // segments + review call
	Kind         string `json:"kind"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
	Input        string `json:"input,omitempty"`
	Text         string `json:"text,omitempty"`
	TurnFrom     int    `json:"turnFrom,omitempty"`
	TurnTo       int    `json:"turnTo,omitempty"`
	Error        string `json:"error,omitempty"`
}

type sessionAuditTurnText struct {
	turn      int
	prompt    string
	reasoning string
	truncated bool
}

// AuditSessionReasoning scores a whole session's reasoning in two passes: each
// segment of consecutive turns is scored with the per-turn classes, then one
// review call judges the cross-turn problems from the structured per-turn
// results. It runs on the dedicated audit model, streams every step through
// onEvent, and never touches the session history or the provider-visible
// prefix. The returned verdict is one-shot evidence: nothing is persisted.
func (c *Controller) AuditSessionReasoning(
	ctx context.Context,
	turns []SessionAuditTurn,
	onEvent func(SessionAuditEvent),
) (SessionAuditTotals, error) {
	var zero SessionAuditTotals
	if c == nil {
		return zero, fmt.Errorf("session audit: session not ready")
	}
	emit := func(ev SessionAuditEvent) {
		if onEvent != nil {
			onEvent(ev)
		}
	}
	c.mu.Lock()
	modelRef := c.audit.model
	resolver := c.audit.providerResolver
	rateCard := c.audit.rateCard
	effort := c.audit.effort
	maxChars := c.audit.maxChars
	c.mu.Unlock()
	p, err := c.resolveStandaloneModel("session reasoning audit", modelRef, resolver)
	if err != nil {
		return zero, err
	}
	segments := buildSessionAuditSegments(turns, maxChars)
	if len(segments) == 0 {
		return zero, fmt.Errorf("session audit: no auditable reasoning in the loaded turns")
	}
	total := len(segments) + 1
	start := time.Now()
	audited := 0
	for _, seg := range segments {
		audited += len(seg)
	}
	results := make([]SessionAuditTurnResult, 0, audited)
	tokens := 0
	cost := 0.0

	for i, seg := range segments {
		if err := ctx.Err(); err != nil {
			return zero, fmt.Errorf("session audit: %w", err)
		}
		index := i + 1
		from, to := seg[0].turn, seg[len(seg)-1].turn
		input := sessionAuditSegmentInput(seg)
		emit(SessionAuditEvent{
			Stage: "segment", Index: index, Total: total, Kind: "request",
			SystemPrompt: auditSegmentPromptContent, Input: input, TurnFrom: from, TurnTo: to,
		})
		callCtx, cancel := context.WithTimeout(ctx, sessionAuditSegmentTimeout)
		res, err := runAuditCall(callCtx, p, auditSegmentPromptContent, input, sessionAuditSegmentMaxTokens, auditRequestEffort(effort),
			auditDeltaEmitter(emit, "segment", index, total, "reasoning"),
			auditDeltaEmitter(emit, "segment", index, total, "text"))
		cancel()
		if err != nil {
			emit(SessionAuditEvent{Stage: "segment", Index: index, Total: total, Kind: "step_failed", TurnFrom: from, TurnTo: to, Error: err.Error()})
			return zero, fmt.Errorf("session audit: segment %d (turns %d-%d): %w", index, from, to, err)
		}
		tokens += auditUsageTokens(res.usage)
		cost += auditCallCost(rateCard, res.usage, modelRef)
		segResults, err := decodeSessionAuditSegment(res.text, seg)
		if err != nil {
			emit(SessionAuditEvent{Stage: "segment", Index: index, Total: total, Kind: "step_failed", TurnFrom: from, TurnTo: to, Error: err.Error()})
			return zero, fmt.Errorf("session audit: segment %d (turns %d-%d): %w", index, from, to, err)
		}
		results = append(results, segResults...)
		emit(SessionAuditEvent{Stage: "segment", Index: index, Total: total, Kind: "step_done", TurnFrom: from, TurnTo: to})
	}

	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("session audit: %w", err)
	}
	reviewInput := sessionAuditReviewInput(results)
	emit(SessionAuditEvent{
		Stage: "review", Index: total, Total: total, Kind: "request",
		SystemPrompt: auditSessionPromptContent, Input: reviewInput,
		TurnFrom: results[0].Turn, TurnTo: results[len(results)-1].Turn,
	})
	reviewCtx, cancelReview := context.WithTimeout(ctx, sessionAuditReviewTimeout)
	reviewRes, err := runAuditCall(reviewCtx, p, auditSessionPromptContent, reviewInput, sessionAuditReviewMaxTokens, auditRequestEffort(effort),
		auditDeltaEmitter(emit, "review", total, total, "reasoning"),
		auditDeltaEmitter(emit, "review", total, total, "text"))
	cancelReview()
	if err != nil {
		emit(SessionAuditEvent{Stage: "review", Index: total, Total: total, Kind: "step_failed", Error: err.Error()})
		return zero, fmt.Errorf("session audit: review: %w", err)
	}
	tokens += auditUsageTokens(reviewRes.usage)
	cost += auditCallCost(rateCard, reviewRes.usage, modelRef)
	verdict, err := decodeSessionAuditReview(reviewRes.text)
	if err != nil {
		emit(SessionAuditEvent{Stage: "review", Index: total, Total: total, Kind: "step_failed", Error: err.Error()})
		return zero, fmt.Errorf("session audit: review: %w", err)
	}
	emit(SessionAuditEvent{Stage: "review", Index: total, Total: total, Kind: "step_done"})

	return SessionAuditTotals{
		Audited:      true,
		ElapsedMs:    time.Since(start).Milliseconds(),
		Score:        verdict.Score,
		Trend:        verdict.Trend,
		Explanation:  verdict.Explanation,
		Issues:       verdict.Issues,
		Turns:        results,
		TurnCount:    len(results),
		SegmentCount: len(segments),
		EvalTokens:   tokens,
		EvalCost:     cost,
	}, nil
}

func auditDeltaEmitter(emit func(SessionAuditEvent), stage string, index, total int, kind string) func(string) {
	return func(chunk string) {
		if chunk == "" {
			return
		}
		emit(SessionAuditEvent{Stage: stage, Index: index, Total: total, Kind: kind, Text: chunk})
	}
}

// buildSessionAuditSegments turns the loaded turns into the exact input each
// audit call will send: turns without reasoning are dropped, the turns are
// split into consecutive groups, and every chain is cut down to that group's
// per-turn allowance. Longer sessions therefore trade per-turn fidelity for a
// bounded call count instead of overflowing one call's input.
func buildSessionAuditSegments(turns []SessionAuditTurn, maxChars int) [][]sessionAuditTurnText {
	if maxChars <= 0 {
		maxChars = reasoningAuditMaxCharsDefault
	}
	cleaned := make([]sessionAuditTurnText, 0, len(turns))
	for i, t := range turns {
		reasoning := strings.TrimSpace(t.Reasoning)
		if reasoning == "" {
			continue
		}
		prompt, cutPrompt := truncateAuditInput(strings.TrimSpace(t.Prompt), sessionAuditTurnPromptMaxChar)
		turn := t.Turn
		if turn <= 0 {
			turn = i + 1
		}
		cleaned = append(cleaned, sessionAuditTurnText{turn: turn, prompt: prompt, reasoning: reasoning, truncated: cutPrompt})
	}
	if len(cleaned) == 0 {
		return nil
	}
	groups := sessionAuditGroupSizes(len(cleaned))
	out := make([][]sessionAuditTurnText, 0, len(groups))
	at := 0
	for _, size := range groups {
		group := cleaned[at : at+size]
		at += size
		out = append(out, cutSessionAuditGroup(group, maxChars))
	}
	return out
}

// cutSessionAuditGroup applies the group's per-turn allowance: a per-turn share
// of sessionAuditCallInputChars, floored so a long session still shows the
// evaluator a real chain, and never above the configured per-turn ceiling.
func cutSessionAuditGroup(group []sessionAuditTurnText, maxChars int) []sessionAuditTurnText {
	share := maxChars
	if quota := sessionAuditCallInputChars / len(group); quota < share {
		share = quota
	}
	if share < sessionAuditMinTurnChars {
		share = sessionAuditMinTurnChars
	}
	if share > maxChars {
		share = maxChars
	}
	out := make([]sessionAuditTurnText, 0, len(group))
	for _, t := range group {
		text, cut := truncateAuditInput(t.reasoning, share)
		t.reasoning = text
		t.truncated = t.truncated || cut
		out = append(out, t)
	}
	return out
}

// sessionAuditGroupSizes splits n turns into consecutive groups of at most
// sessionAuditSegmentMaxTurns, never more than sessionAuditMaxSegments groups.
func sessionAuditGroupSizes(n int) []int {
	groups := (n + sessionAuditSegmentMaxTurns - 1) / sessionAuditSegmentMaxTurns
	groups = min(groups, sessionAuditMaxSegments)
	groups = max(groups, 1)
	sizes := make([]int, 0, groups)
	base, extra := n/groups, n%groups
	for i := range groups {
		size := base
		if i < extra {
			size++
		}
		if size > 0 {
			sizes = append(sizes, size)
		}
	}
	return sizes
}

func sessionAuditSegmentInput(seg []sessionAuditTurnText) string {
	var b strings.Builder
	for i, t := range seg {
		if i > 0 {
			b.WriteString("\n")
		}
		prompt := t.prompt
		if prompt == "" {
			prompt = "（未提供）"
		}
		fmt.Fprintf(&b, "[轮 %d] 用户要求：%s\n思考过程：\n%s\n", t.turn, prompt, t.reasoning)
	}
	return b.String()
}

// sessionAuditReviewInput renders the per-turn results as the review pass's
// input. It carries numbers, conclusions, and quoted excerpts only — never the
// reasoning text itself, which keeps the review call small and bounded.
func sessionAuditReviewInput(results []SessionAuditTurnResult) string {
	var b strings.Builder
	for _, r := range results {
		quotes := make([]string, 0, len(r.Findings))
		for _, f := range r.Findings {
			if q := strings.TrimSpace(f.Quote); q != "" {
				quotes = append(quotes, fmt.Sprintf("%q", q))
			}
		}
		counts := sessionAuditCountText(r)
		conclusion := strings.TrimSpace(r.Conclusion)
		if conclusion == "" {
			conclusion = "（未返回结论）"
		}
		fmt.Fprintf(&b, "turn %d | score %.2f | %s | conclusion %s | prior_conflict %q | quotes [%s]\n",
			r.Turn, r.Score, counts, conclusion, strings.TrimSpace(r.PriorConflict), strings.Join(quotes, ", "))
	}
	return b.String()
}

func sessionAuditCountText(r SessionAuditTurnResult) string {
	pairs := []struct {
		name  string
		count int
	}{
		{"contradiction", r.Contradiction},
		{"factual_error", r.FactualError},
		{"invalid_inference", r.InvalidInference},
		{"redundancy", r.Redundancy},
		{"instruction_drift", r.InstructionDrift},
		{"omission", r.Omission},
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p.count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", p.name, p.count))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

type sessionAuditTurnVerdict struct {
	Turn             int                  `json:"turn"`
	Score            float64              `json:"score"`
	Contradiction    int                  `json:"contradiction"`
	FactualError     int                  `json:"factual_error"`
	InvalidInference int                  `json:"invalid_inference"`
	Redundancy       int                  `json:"redundancy"`
	InstructionDrift int                  `json:"instruction_drift"`
	Omission         int                  `json:"omission"`
	Conclusion       string               `json:"conclusion"`
	PriorConflict    string               `json:"prior_conflict"`
	Explanation      string               `json:"explanation"`
	Findings         []event.AuditFinding `json:"findings"`
}

// decodeSessionAuditSegment maps one segment's verdict onto the audited turns.
// A turn the evaluator omitted is kept as an explicit "no result" row instead
// of being silently dropped, so the review pass and the UI see the gap.
func decodeSessionAuditSegment(text string, seg []sessionAuditTurnText) ([]SessionAuditTurnResult, error) {
	var doc struct {
		Turns []sessionAuditTurnVerdict `json:"turns"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &doc); err != nil {
		return nil, fmt.Errorf("decode verdict: %w", err)
	}
	if len(doc.Turns) == 0 {
		return nil, fmt.Errorf("decode verdict: no turn rows")
	}
	byTurn := make(map[int]sessionAuditTurnVerdict, len(doc.Turns))
	for _, v := range doc.Turns {
		byTurn[v.Turn] = v
	}
	out := make([]SessionAuditTurnResult, 0, len(seg))
	matched := 0
	for _, t := range seg {
		v, ok := byTurn[t.turn]
		if !ok {
			out = append(out, SessionAuditTurnResult{Turn: t.turn, Explanation: "该轮未返回评审结果", Truncated: t.truncated})
			continue
		}
		matched++
		out = append(out, sessionAuditTurnResultFrom(t.turn, v, t.truncated))
	}
	if matched == 0 {
		return nil, fmt.Errorf("decode verdict: turn numbers do not match the audited turns")
	}
	return out, nil
}

func sessionAuditTurnResultFrom(turn int, v sessionAuditTurnVerdict, truncated bool) SessionAuditTurnResult {
	score := v.Score
	if score < 0 || score > 1 {
		score = sessionAuditScoreFromCounts(v.Contradiction, v.FactualError, v.InvalidInference, v.Redundancy, v.InstructionDrift, v.Omission)
	}
	return SessionAuditTurnResult{
		Turn:             turn,
		Score:            score,
		Contradiction:    v.Contradiction,
		FactualError:     v.FactualError,
		InvalidInference: v.InvalidInference,
		Redundancy:       v.Redundancy,
		InstructionDrift: v.InstructionDrift,
		Omission:         v.Omission,
		Issues:           v.Contradiction + v.FactualError + v.InvalidInference + v.Redundancy + v.InstructionDrift + v.Omission,
		Conclusion:       strings.TrimSpace(v.Conclusion),
		PriorConflict:    strings.TrimSpace(v.PriorConflict),
		Explanation:      strings.TrimSpace(v.Explanation),
		Findings:         v.Findings,
		Truncated:        truncated,
	}
}

// sessionAuditScoreFromCounts applies the documented six-class formula. It is
// the fallback when the evaluator reports a score outside [0,1].
func sessionAuditScoreFromCounts(contradiction, factual, invalid, redundancy, drift, omission int) float64 {
	raw := 1 -
		0.15*float64(contradiction) -
		0.22*float64(factual) -
		0.18*float64(invalid) -
		0.15*float64(omission) -
		0.12*float64(drift) -
		0.05*float64(redundancy)
	if raw < 0 {
		return 0
	}
	return math.Round(raw*100) / 100
}

type sessionAuditReviewVerdict struct {
	Score       float64             `json:"score"`
	Trend       string              `json:"trend"`
	Explanation string              `json:"explanation"`
	Issues      []SessionAuditIssue `json:"issues"`
}

func decodeSessionAuditReview(text string) (sessionAuditReviewVerdict, error) {
	var v sessionAuditReviewVerdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &v); err != nil {
		return sessionAuditReviewVerdict{}, fmt.Errorf("decode verdict: %w", err)
	}
	if v.Score < 0 || v.Score > 1 {
		return sessionAuditReviewVerdict{}, fmt.Errorf("verdict score %g out of range", v.Score)
	}
	v.Score = math.Round(v.Score*100) / 100
	switch v.Trend {
	case "improving", "stable", "degrading":
	default:
		v.Trend = "stable"
	}
	v.Explanation = strings.TrimSpace(v.Explanation)
	issues := make([]SessionAuditIssue, 0, len(v.Issues))
	for _, issue := range v.Issues {
		issue.Type = strings.TrimSpace(issue.Type)
		issue.Note = strings.TrimSpace(issue.Note)
		issue.Quote = strings.TrimSpace(issue.Quote)
		if issue.Type == "" || len(issue.Turns) == 0 {
			continue
		}
		sort.Ints(issue.Turns)
		issues = append(issues, issue)
		if len(issues) == sessionAuditMaxIssues {
			break
		}
	}
	v.Issues = issues
	return v, nil
}

func auditUsageTokens(usage *provider.Usage) int {
	if usage == nil {
		return 0
	}
	return usage.TotalTokens
}

// auditCallCost converts one evaluator call's usage into spend, using the same
// rate card as the model's normal completions.
func auditCallCost(rateCard func() (billing.RateCard, bool), usage *provider.Usage, modelRef string) float64 {
	if usage == nil || rateCard == nil {
		return 0
	}
	card, ok := rateCard()
	if !ok || (card.Currency == "" && card.Input == 0 && card.Output == 0) {
		return 0
	}
	q := billing.BuildQuote(billing.QuoteInput{
		Usage:    usageTokensForAudit(usage),
		Rates:    card,
		ModelRef: modelRef,
	})
	return q.LegacyCostFloat()
}
