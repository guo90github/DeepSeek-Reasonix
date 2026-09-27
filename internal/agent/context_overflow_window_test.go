package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// windowStrictProvider counts three characters per token where the agent's cold
// estimate assumes four, and rejects any request whose prompt plus the output it
// reserved exceeds the window. That is the 2026-09-27 shape: the prompt itself
// fit the window, the reservation tipped the request over, and the agent's own
// estimate never crossed its ceiling — so no local admission gate fired and the
// rejection only arrived from the provider.
type windowStrictProvider struct {
	mu     sync.Mutex
	window int
	// alwaysOverflow reports every prompt as at least the window, so no summary
	// form can ever land and only the lossy rescue is left.
	alwaysOverflow bool
	// ledger is every request in order: its dense prompt+completion, whether it
	// was a summary request, and whether the provider accepted it.
	ledger []strictRequest
}

type strictRequest struct {
	dense   int
	summary bool
	ok      bool
}

func (p *windowStrictProvider) Name() string { return "window-strict" }

func (p *windowStrictProvider) ContextBudgetPolicy() provider.ContextBudgetPolicy {
	return provider.ContextBudgetPolicy{
		WindowMode: provider.ContextWindowShared, AutoOutputTokens: 8192, MaxOutputTokens: 8192,
		LimitMode: provider.OutputLimitOmitWhenSafe,
	}
}

func strictDense(req provider.Request) int {
	chars, _, _ := requestCalibrationTextShape(req, provider.SharedWindowInputPolicy{})
	return int(chars) / 3
}

func (p *windowStrictProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prompt := strictDense(req)
	if p.alwaysOverflow {
		prompt = max(prompt, p.window)
	}
	completion := req.MaxTokens
	if completion <= 0 {
		completion = 8192
	}
	entry := strictRequest{dense: prompt + completion, summary: isSummaryRequest(req)}
	if entry.dense > p.window {
		p.ledger = append(p.ledger, entry)
		body := fmt.Sprintf(deepSeekOverflowBody, p.window, entry.dense, prompt, completion)
		limit := provider.ParseContextLimitError(&provider.APIError{Provider: p.Name(), Status: 400, Body: body})
		if limit == nil {
			return nil, fmt.Errorf("test body did not parse as a context limit: %s", body)
		}
		return nil, limit
	}
	entry.ok = true
	p.ledger = append(p.ledger, entry)
	text := "ok"
	if entry.summary {
		text = "- goal: keep going\n- pending: continue"
	}
	return chunks(
		provider.Chunk{Type: provider.ChunkText, Text: text},
		provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{
			PromptTokens: prompt, CompletionTokens: 8, TotalTokens: prompt + 8, RequestCount: 1,
		}},
		provider.Chunk{Type: provider.ChunkDone},
	), nil
}

// windowStrictSession fills a transcript with assistant text only, so the free
// prune pass has no tool result to reclaim and the session can only leave the
// window by folding or truncating.
func windowStrictSession(chars int) *Session {
	sess := NewSession("sys")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "standing constraint: keep the public API stable"})
	body := strings.Repeat("alpha beta gamma delta ", 20)
	for written := 0; written < chars; written += len(body) {
		sess.Add(provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("step %d: %s", written, body)})
		sess.Add(provider.Message{Role: provider.RoleUser, Content: "continue"})
	}
	return sess
}

// audit checks the incident's conditions directly: the provider never accepted a
// request over its window, a rejection was never answered by resending the same
// shape, and the turn still went out afterwards.
func (p *windowStrictProvider) audit(t *testing.T, window int) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	t.Logf("window=%d ledger=%+v", window, p.ledger)
	rejected, wentOutAfter := 0, false
	for _, entry := range p.ledger {
		if entry.ok {
			if entry.dense > window {
				t.Errorf("provider accepted a %d-token request against a %d window", entry.dense, window)
			}
			if rejected > 0 {
				wentOutAfter = true
			}
			continue
		}
		rejected++
	}
	if rejected == 0 {
		t.Fatal("the fixture never reached the over-window shape: nothing was rejected")
	}
	if !wentOutAfter {
		t.Error("no request was accepted after the rejection: the turn never went out")
	}
	if !p.shrankAfterRejectionLocked() {
		t.Error("the rejected shape was repeated instead of re-planned smaller: the summarizer's own input never shrank")
	}
}

// shrankAfterRejectionLocked reports whether a summary request smaller than the
// rejected one was accepted afterwards: the ladder answered the overflow with a
// smaller fold rather than resending the same bytes. Caller holds p.mu.
func (p *windowStrictProvider) shrankAfterRejectionLocked() bool {
	rejectedDense := 0
	for _, entry := range p.ledger {
		switch {
		case !entry.ok:
			if entry.summary {
				rejectedDense = entry.dense
			}
		case rejectedDense > 0 && entry.summary && entry.dense < rejectedDense:
			return true
		}
	}
	return false
}

// The provider rejects prompt-plus-reservation while the agent's own estimate
// stays under its ceiling. The turn must still start, and the rejection must be
// answered by a smaller view — the 2026-09-27 session died because the rejection
// repeated for hours with nothing shrinking.
func TestOverWindowTurnShrinksInsteadOfRepeatingTheRequest(t *testing.T) {
	const window = 30_000
	prov := &windowStrictProvider{window: window}
	sess := windowStrictSession(105_000)
	a := New(prov, tool.NewRegistry(), sess, Options{
		ContextWindow: window, CompactRatio: 0.8, RecentKeep: 2, ArchiveDir: t.TempDir(),
	}, event.Discard)
	if est, hard := a.estimatedVisibleRequestTokens(sess.Snapshot()), a.hardInputCeiling(); est >= hard {
		t.Fatalf("fixture estimates %d tokens against a %d ceiling; the local gate would fire first", est, hard)
	}

	if err := a.Run(context.Background(), "continue"); err != nil {
		t.Fatalf("Run = %v, want a started turn", err)
	}
	prov.audit(t, window)

	// The fold never fabricates a digest from a failed summary, so the recovered
	// view has to say something was shortened.
	if visible := a.modelVisibleMessages(); !visibleCarriesAShrinkNote(visible) {
		t.Fatalf("no shrink note in the recovered view: %q", previewOf(visible))
	}
}

// No summary form can land and the view is over the hard ceiling, so only the
// lossy truncation can let the turn leave — on the automatic path, and with a
// record the user can see rather than a log line.
func TestOverCeilingTurnLeavesAVisibleTruncation(t *testing.T) {
	const window = 30_000
	prov := &windowStrictProvider{window: window, alwaysOverflow: true}
	sess := windowStrictSession(140_000)
	a := New(prov, tool.NewRegistry(), sess, Options{
		ContextWindow: window, CompactRatio: 0.8, RecentKeep: 2, ArchiveDir: t.TempDir(),
	}, event.Discard)
	if est, hard := a.estimatedVisibleRequestTokens(sess.Snapshot()), a.hardInputCeiling(); est < hard {
		t.Fatalf("fixture estimates %d tokens against a %d ceiling; the rescue is not exercised", est, hard)
	}
	var truncated int
	a.svc.sink = event.FuncSink(func(e event.Event) {
		if e.Kind == event.ContextMaintenanceEvent && e.Maintenance != nil &&
			e.Maintenance.Action == maintenanceActionTruncate && e.Maintenance.Status == "applied" {
			truncated++
		}
	})

	// Every request is rejected, so the turn cannot complete; what must survive is
	// the shrink, recorded where a reader can find it.
	_ = a.Run(context.Background(), "continue")
	if truncated == 0 {
		t.Fatalf("no applied truncation on the automatic path; receipts seen: %+v", a.sess.compactionState.LastReceipt)
	}
	if visible := a.modelVisibleMessages(); !visibleCarriesAShrinkNote(visible) {
		t.Fatalf("no shrink note in the recovered view: %q", previewOf(visible))
	}
}

func visibleCarriesAShrinkNote(visible []provider.Message) bool {
	for _, m := range visible {
		if strings.Contains(m.Content, "truncated to fit the context window") ||
			strings.Contains(m.Content, elidedToolResultPrefix) ||
			isCompactionSummary(m) {
			return true
		}
	}
	return false
}

func previewOf(visible []provider.Message) string {
	var b strings.Builder
	for i, m := range visible {
		if i == 6 {
			break
		}
		fmt.Fprintf(&b, "[%s %d chars] ", m.Role, len(m.Content))
	}
	return b.String()
}
