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
// thinking chain against six failure classes and return a structured JSON
// verdict (see audit_system_prompt.md, which is the verbatim system prompt).
// The evaluator is deterministic (temperature 0) and never thinks.
var reasoningAuditSystemPrompt = auditSystemPromptContent

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
		FactualError     int                  `json:"factual_error"`
		InvalidInference int                  `json:"invalid_inference"`
		Redundancy       int                  `json:"redundancy"`
		InstructionDrift int                  `json:"instruction_drift"`
		Omission         int                  `json:"omission"`
		Hallucination    int                  `json:"hallucination"` // legacy four-class output
		Explanation      string               `json:"explanation"`
		Findings         []event.AuditFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out.String()), &verdict); err != nil {
		return zero, fmt.Errorf("reasoning audit: decode verdict: %w", err)
	}
	if verdict.Score < 0 || verdict.Score > 1 {
		return zero, fmt.Errorf("reasoning audit: verdict score %g out of range", verdict.Score)
	}
	issues := verdict.Contradiction + verdict.FactualError + verdict.InvalidInference +
		verdict.Redundancy + verdict.InstructionDrift + verdict.Omission
	totals := event.ReasoningAuditTotals{
		Audited:          true,
		ElapsedMs:        elapsed,
		Contradiction:    verdict.Contradiction,
		FactualError:     verdict.FactualError,
		InvalidInference: verdict.InvalidInference,
		Redundancy:       verdict.Redundancy,
		InstructionDrift: verdict.InstructionDrift,
		Omission:         verdict.Omission,
		Hallucination:    verdict.Hallucination,
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
