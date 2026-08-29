package control

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/billing"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

const (
	// reasoningAuditMaxTokens caps the evaluator's response; the verdict is a
	// short JSON object, never a long critique.
	reasoningAuditMaxTokens = 1200
	// reasoningAuditMaxChars caps the reasoning excerpt sent to the evaluator so
	// an unbounded thinking chain cannot turn the utility into a context dump.
	reasoningAuditMaxChars = 8000
	// reasoningAuditTimeout bounds the independent evaluator call so a slow
	// audit model can never stall the desktop.
	reasoningAuditTimeout = 90 * time.Second
)

// reasoningAuditSystemPrompt instructs the standalone evaluator to score a
// thinking chain against four failure classes and return a compact JSON
// verdict. The evaluator is deterministic (temperature 0) and never thinks.
const reasoningAuditSystemPrompt = `你是一名称职的思考链质量审阅者。对给定的模型思考过程，按以下四类计分并输出 JSON：
1. contradiction：思考过程中互相矛盾或前后抵消的中间结论个数
2. hallucination：缺乏依据或与已知事实冲突的断言个数
3. redundancy：重复复述、来回兜圈造成的冗余片段个数
4. instruction_drift：偏离用户指令或目标的做法个数
5. explanation：用 1-2 句说明评分依据，指出最主要的问题或优点，使用用户使用的语言
6. findings：可选数组，列出被指出的具体问题，每项 {"type":"contradiction|hallucination|redundancy|instruction_drift","quote":"被审计链中对应原句"}，每类最多 2 条；无则省略
输出严格为单个 JSON 对象，不要任何解释或代码块：
{"score":0.0,"contradiction":0,"hallucination":0,"redundancy":0,"instruction_drift":0,"explanation":"评分依据","findings":[]}
score 是 0..1 的综合质量分，0 为最差，1 为最优。若思考链很短或很干净，对应项为 0，score 接近 1。`

// AnalyzeReasoning scores one turn's reasoning chain with the dedicated audit
// model (AuditModel), which is deliberately independent of the session model.
// It returns a content-free summary; the reasoning text itself is never part of
// the result and never enters the session history or provider-visible prefix.
// It is a shadow utility: an error or evaluator failure never affects the turn.
func (c *Controller) AnalyzeReasoning(ctx context.Context, reasoning string) (event.ReasoningAuditTotals, error) {
	return c.AuditStream(ctx, reasoning, nil, nil, nil)
}

// AuditStream runs one audit and streams its progress. onRequest fires once
// with the exact request params before the model call (truncated reports whether
// the audited input was cut to reasoningAuditMaxChars); onReasoning fires per
// thinking delta; onText fires per verdict-text delta. Text deltas stream live
// even when the audit model runs with effort disabled (the final JSON verdict
// still arrives as ChunkText), so the frontend renders the model's output as it
// is produced instead of a black-box single result.
func (c *Controller) AuditStream(
	ctx context.Context,
	reasoning string,
	onRequest func(systemPrompt, input string, truncated bool),
	onReasoning func(string),
	onText func(string),
) (event.ReasoningAuditTotals, error) {
	var zero event.ReasoningAuditTotals
	if c == nil {
		return zero, fmt.Errorf("reasoning audit: session not ready")
	}
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return zero, fmt.Errorf("reasoning audit: empty reasoning")
	}
	c.mu.Lock()
	audited := c.audit.enabled
	modelRef := c.audit.model
	resolver := c.audit.providerResolver
	rateCard := c.audit.rateCard
	effort := c.audit.effort
	c.mu.Unlock()
	if !audited {
		return zero, fmt.Errorf("reasoning audit: disabled")
	}
	p, err := c.resolveStandaloneModel("reasoning audit", modelRef, resolver)
	if err != nil {
		return zero, err
	}
	truncated := false
	if len(reasoning) > reasoningAuditMaxChars {
		reasoning = reasoning[:reasoningAuditMaxChars]
		truncated = true
	}
	if onRequest != nil {
		onRequest(reasoningAuditSystemPrompt, reasoning, truncated)
	}

	start := time.Now()
	requestCtx, cancel := context.WithTimeout(ctx, reasoningAuditTimeout)
	defer cancel()
	stream, err := p.Stream(requestCtx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: reasoningAuditSystemPrompt},
			{Role: provider.RoleUser, Content: reasoning},
		},
		Temperature:    provider.TemperaturePtr(0),
		MaxTokens:      reasoningAuditMaxTokens,
		EffortOverride: auditRequestEffort(effort),
	})
	if err != nil {
		return zero, fmt.Errorf("reasoning audit: %w", err)
	}
	var out strings.Builder
	var usage *provider.Usage
	for chunk := range stream {
		switch chunk.Type {
		case provider.ChunkText:
			out.WriteString(chunk.Text)
			if onText != nil {
				onText(chunk.Text)
			}
		case provider.ChunkReasoning:
			if onReasoning != nil {
				onReasoning(chunk.Text)
			}
		case provider.ChunkUsage:
			if chunk.Usage != nil {
				usage = chunk.Usage
			}
		case provider.ChunkError:
			if chunk.Err != nil {
				return zero, fmt.Errorf("reasoning audit: %w", chunk.Err)
			}
		}
	}
	elapsed := time.Since(start).Milliseconds()

	var verdict struct {
		Score            float64              `json:"score"`
		Contradiction    int                  `json:"contradiction"`
		Hallucination    int                  `json:"hallucination"`
		Redundancy       int                  `json:"redundancy"`
		InstructionDrift int                  `json:"instruction_drift"`
		Explanation      string               `json:"explanation"`
		Findings         []event.AuditFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out.String()), &verdict); err != nil {
		return zero, fmt.Errorf("reasoning audit: decode verdict: %w", err)
	}
	if verdict.Score < 0 || verdict.Score > 1 {
		return zero, fmt.Errorf("reasoning audit: verdict score %g out of range", verdict.Score)
	}
	issues := verdict.Contradiction + verdict.Hallucination + verdict.Redundancy + verdict.InstructionDrift
	totals := event.ReasoningAuditTotals{
		Audited:          true,
		ElapsedMs:        elapsed,
		Contradiction:    verdict.Contradiction,
		Hallucination:    verdict.Hallucination,
		Redundancy:       verdict.Redundancy,
		InstructionDrift: verdict.InstructionDrift,
		Issues:           issues,
		Score:            verdict.Score,
		Explanation:      verdict.Explanation,
		Findings:         verdict.Findings,
	}
	if usage != nil {
		totals.EvalTokens = usage.TotalTokens
		if rateCard != nil {
			if card, ok := rateCard(); ok && (card.Currency != "" || card.Input > 0 || card.Output > 0) {
				q := billing.BuildQuote(billing.QuoteInput{
					Usage:    usageTokensForAudit(usage),
					Rates:    card,
					ModelRef: modelRef,
				})
				totals.EvalCost = q.LegacyCostFloat()
			}
		}
	}
	return totals, nil
}

// auditRequestEffort maps the configured audit reasoning-depth to the provider's
// EffortOverride. Empty and "off" mean the audit model does not think (a
// deterministic score); explicit low/medium/high pass through to the provider
// adapter, which validates them against the endpoint's effort vocabulary.
func auditRequestEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "", "off", "disabled", "none":
		return "disabled"
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return "disabled"
	}
}

// usageTokensForAudit maps a provider usage record to the billing token shape
// used for cost estimation. The audit call is billed from the same rates as
// the audit model's normal completions.
func usageTokensForAudit(u *provider.Usage) billing.UsageTokens {
	return billing.UsageTokens{
		PromptTokens:           u.PromptTokens,
		CompletionTokens:       u.CompletionTokens,
		CacheHitTokens:         u.CacheHitTokens,
		CacheMissTokens:        u.CacheMissTokens,
		CacheWriteTokens:       u.CacheWriteTokens,
		CacheWriteBilledTokens: u.CacheWriteBilledTokens,
		Estimated:              u.Estimated,
	}
}
