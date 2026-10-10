package boot

// The host's off-path decision is a launch decision, not an edit: with
// shell_async=fast the slow call starts as a job, and the tool call the provider
// sees on the next round still carries exactly the arguments the model streamed.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

const byteExactSlowArgs = `{"command":"sleep 2"}`

type byteExactProvider struct {
	mu    sync.Mutex
	round int
	reqs  []provider.Request
}

func (*byteExactProvider) Name() string { return "boot-loop-bytes" }

func (p *byteExactProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 3)
	switch round {
	case 1:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "slow", Name: "bash", Arguments: byteExactSlowArgs}}
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "later", Name: "read_file", Arguments: `{"path":"note.txt"}`}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *byteExactProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// toolCallArgsFor returns the arguments the provider sees for one tool call.
func toolCallArgsFor(req provider.Request, callID string) (string, bool) {
	for _, msg := range req.Messages {
		for _, call := range msg.ToolCalls {
			if call.ID == callID {
				return call.Arguments, true
			}
		}
	}
	return "", false
}

// TestEffectPromotionKeepsTheToolCallArgumentsOnTheWire pins the invariant the
// out-of-band channel exists for: promotion may start a shell call as a job
// without editing the call itself, so the next request carries the model's own
// arguments byte for byte.
func TestEffectPromotionKeepsTheToolCallArgumentsOnTheWire(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &byteExactProvider{}
	provider.Register("boot-loop-bytes", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "note.txt", "alpha\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
shell_async = "fast"

[[providers]]
name = "test-model"
kind = "boot-loop-bytes"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "run the slow thing and read the note"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	reqs := rec.requests()
	if len(reqs) < 2 {
		t.Fatalf("run stopped after %d rounds", len(reqs))
	}

	slow, ok := toolResultFor(reqs[1], "slow")
	if !ok || !strings.Contains(slow, "Started background job") {
		t.Fatalf("the slow call did not start as a job: ok=%v %q", ok, slow)
	}
	got, ok := toolCallArgsFor(reqs[1], "slow")
	if !ok {
		t.Fatal("the slow tool call is missing from the second request")
	}
	if got != byteExactSlowArgs {
		t.Fatalf("the tool call on the wire changed: %q, want %q", got, byteExactSlowArgs)
	}
	if strings.Contains(got, "run_in_background") {
		t.Fatalf("the host's launch decision leaked into the call: %q", got)
	}
}
