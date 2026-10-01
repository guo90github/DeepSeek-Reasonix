package boot

// Effect test for the async closure: a backgrounded check must become a usable
// receipt once its job is collected — the collected exit status is the proof.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type jobReceiptProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *jobReceiptProvider) Name() string { return "boot-job-receipt" }

func (p *jobReceiptProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	round := len(p.reqs)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 2)
	switch round {
	case 1:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID: "s1", Name: "bash", Arguments: `{"command":"node --check note.js","run_in_background":true}`,
		}}
	case 2:
		// The first job of a session is bash-1; wait makes the collection
		// deterministic instead of racing a freshly started job.
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID: "c1", Name: "wait", Arguments: `{"job_ids":["bash-1"]}`,
		}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *jobReceiptProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

func TestEffectCollectedBackgroundCheckBecomesAReceipt(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &jobReceiptProvider{}
	provider.Register("boot-job-receipt", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "note.js", "const answer = 42\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "boot-job-receipt"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "check the note in the background and collect it"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	reqs := rec.requests()
	if len(reqs) < 3 {
		t.Fatalf("run stopped after %d rounds", len(reqs))
	}
	last := reqs[len(reqs)-1]
	started, ok := toolResultFor(last, "s1")
	if !ok || !strings.Contains(started, "Started background job") {
		t.Fatalf("the check was not started as a job: ok=%v %q", ok, started)
	}
	collected, ok := toolResultFor(last, "c1")
	if !ok {
		t.Fatal("the collection produced no result")
	}
	if !strings.Contains(collected, "cite it to sign this off") {
		t.Fatalf("collecting the finished check did not produce a receipt: %q", collected)
	}
	if !strings.Contains(collected, "node --check note.js") {
		t.Fatalf("the receipt must name the command it belongs to: %q", collected)
	}
}
