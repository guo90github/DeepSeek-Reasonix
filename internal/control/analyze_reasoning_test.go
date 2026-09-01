package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type reasoningAuditTestProvider struct {
	lastRequest provider.Request
	verdict     string
	usage       *provider.Usage
}

func (p *reasoningAuditTestProvider) Name() string { return "audit-test" }

func (p *reasoningAuditTestProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.lastRequest = req
	out := make(chan provider.Chunk, 3)
	go func() {
		defer close(out)
		select {
		case out <- provider.Chunk{Type: provider.ChunkText, Text: p.verdict}:
		case <-ctx.Done():
			return
		}
		if p.usage != nil {
			select {
			case out <- provider.Chunk{Type: provider.ChunkUsage, Usage: p.usage}:
			case <-ctx.Done():
				return
			}
		}
		out <- provider.Chunk{Type: provider.ChunkDone}
	}()
	return out, nil
}

func auditTestController(t *testing.T, stub *reasoningAuditTestProvider) *Controller {
	t.Helper()
	c := &Controller{
		// The session model must never be the auditor: only the dedicated
		// auditModel ref is resolved.
		modelRef: "session/model",
		audit: auditConfig{
			model:   "audit/qwen",
			enabled: true,
			providerResolver: func(ref string) (provider.Provider, error) {
				return stub, nil
			},
			rateCard: func() (billing.RateCard, bool) {
				return billing.RateCard{Input: 1, Output: 1, Currency: "USD"}, true
			},
		},
		sink: event.Discard,
	}
	return c
}

func TestAnalyzeReasoningRunsOnDedicatedModel(t *testing.T) {
	stub := &reasoningAuditTestProvider{verdict: `{"score":0.4,"contradiction":1,"factual_error":2,"invalid_inference":1,"redundancy":3,"instruction_drift":0,"omission":1}`}
	c := auditTestController(t, stub)
	got, err := c.AnalyzeReasoning(context.Background(), "先假设 A，再否定 A，然后又重复假设")
	if err != nil {
		t.Fatalf("AnalyzeReasoning: %v", err)
	}
	if got.Score != 0.4 || got.Contradiction != 1 || got.FactualError != 2 || got.InvalidInference != 1 ||
		got.Redundancy != 3 || got.InstructionDrift != 0 || got.Omission != 1 {
		t.Fatalf("totals = %+v, want score 0.4 / 1 / 2 / 1 / 3 / 0 / 1", got)
	}
	if got.Issues != 8 {
		t.Fatalf("issues = %d, want 8", got.Issues)
	}
	if !got.Audited {
		t.Fatal("audited = false, want true")
	}
	if stub.lastRequest.EffortOverride != "disabled" {
		t.Fatalf("EffortOverride = %q, want disabled (the auditor must never think)", stub.lastRequest.EffortOverride)
	}
	if stub.lastRequest.Temperature == nil || *stub.lastRequest.Temperature != 0 {
		t.Fatalf("temperature = %v, want 0", stub.lastRequest.Temperature)
	}
	if len(stub.lastRequest.Messages) != 2 || stub.lastRequest.Messages[0].Role != provider.RoleSystem {
		t.Fatalf("request messages = %+v", stub.lastRequest.Messages)
	}
	if !strings.Contains(stub.lastRequest.Messages[1].Content, "先假设 A") {
		t.Fatalf("reasoning not passed to evaluator: %q", stub.lastRequest.Messages[1].Content)
	}
}

func TestAnalyzeReasoningGatedByAuditEnabled(t *testing.T) {
	stub := &reasoningAuditTestProvider{verdict: `{"score":0.9}`}
	disabled := &Controller{audit: auditConfig{model: "audit/qwen", enabled: false, providerResolver: func(string) (provider.Provider, error) { return stub, nil }}}
	if _, err := disabled.AnalyzeReasoning(context.Background(), "hi"); err == nil {
		t.Fatal("expected disabled error")
	}
	unconfigured := &Controller{audit: auditConfig{enabled: true}}
	if _, err := unconfigured.AnalyzeReasoning(context.Background(), "hi"); err == nil {
		t.Fatal("expected unconfigured-model error")
	}
	noResolver := &Controller{audit: auditConfig{enabled: true, model: "audit/qwen"}}
	if _, err := noResolver.AnalyzeReasoning(context.Background(), "hi"); err == nil {
		t.Fatal("expected missing-resolver error")
	}
}

func TestAnalyzeReasoningRejectsEmptyAndBadVerdict(t *testing.T) {
	stub := &reasoningAuditTestProvider{verdict: `{"score":0.9}`}
	c := auditTestController(t, stub)
	if _, err := c.AnalyzeReasoning(context.Background(), "   "); err == nil {
		t.Fatal("expected empty-reasoning error")
	}
	bad := &reasoningAuditTestProvider{verdict: "not json"}
	c2 := auditTestController(t, bad)
	if _, err := c2.AnalyzeReasoning(context.Background(), "some reasoning"); err == nil {
		t.Fatal("expected decode error")
	}
	oob := &reasoningAuditTestProvider{verdict: `{"score":5}`}
	c3 := auditTestController(t, oob)
	if _, err := c3.AnalyzeReasoning(context.Background(), "some reasoning"); err == nil {
		t.Fatal("expected out-of-range score error")
	}
}

func TestAnalyzeReasoningCapturesUsageAndCost(t *testing.T) {
	stub := &reasoningAuditTestProvider{
		verdict: `{"score":0.5}`,
		usage:   &provider.Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500},
	}
	c := auditTestController(t, stub)
	got, err := c.AnalyzeReasoning(context.Background(), "some reasoning")
	if err != nil {
		t.Fatalf("AnalyzeReasoning: %v", err)
	}
	if got.EvalTokens != 1500 {
		t.Fatalf("EvalTokens = %d, want 1500", got.EvalTokens)
	}
	// Input $1/1M * 1000 + output $1/1M * 500 = $0.0015.
	if got.EvalCost < 0.0014 || got.EvalCost > 0.0016 {
		t.Fatalf("EvalCost = %g, want ~0.0015", got.EvalCost)
	}
}

func TestAuditRequestEffortMapping(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"", "disabled"},
		{"off", "disabled"},
		{"disabled", "disabled"},
		{"none", "disabled"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"HIGH", "high"},
		{"bogus", "disabled"},
	} {
		if got := auditRequestEffort(tc.in); got != tc.want {
			t.Fatalf("auditRequestEffort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
