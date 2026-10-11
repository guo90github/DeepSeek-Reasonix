package boot

// The fan-out hint at the real boundary: once the tier registers parallel_calls,
// a long read-only turn is told that the host can run those calls itself. With
// the tier off the same turn hears the older words only.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

const hintReads = 6

type planExecHintProvider struct {
	mu    sync.Mutex
	round int
	last  provider.Request
}

func (*planExecHintProvider) Name() string { return "boot-plan-exec-hint" }

func (p *planExecHintProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.last = req
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 2)
	if round <= hintReads {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID:        fmt.Sprintf("read-%d", round),
			Name:      "read_file",
			Arguments: fmt.Sprintf(`{"path":"note%d.txt"}`, round),
		}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *planExecHintProvider) finalMessages() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var b strings.Builder
	for _, msg := range p.last.Messages {
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// runHintTurn runs one long single-call read-only turn with the given tier and
// returns everything the provider saw in its last request.
func runHintTurn(t *testing.T, tier string) string {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	for i := 1; i <= hintReads; i++ {
		writeFile(t, dir, fmt.Sprintf("note%d.txt", i), fmt.Sprintf("body-%d\n", i))
	}

	rec := &planExecHintProvider{}
	kind := fmt.Sprintf("boot-plan-exec-hint-%s-%s", tier, t.Name())
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
plan_execute = "`+tier+`"

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "read every note file"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rec.finalMessages()
}

// TestEffectPlanExecuteHintFollowsTheTier: the escalation names the host-call
// tool only when the run registered it, and the shared sentence is unchanged.
func TestEffectPlanExecuteHintFollowsTheTier(t *testing.T) {
	off := runHintTurn(t, "off")
	on := runHintTurn(t, "on")

	if !strings.Contains(off, "Host fan-out hint") {
		t.Fatalf("the long read-only turn never got the fan-out hint:\n%s", off)
	}
	if strings.Contains(off, "parallel_calls") {
		t.Fatalf("the off turn was offered the host-call tool:\n%s", off)
	}
	if !strings.Contains(on, "parallel_calls") {
		t.Fatalf("the on turn never heard about the host-call tool:\n%s", on)
	}
	if !strings.Contains(on, "Host fan-out hint") {
		t.Fatalf("the on turn lost the shared fan-out sentence:\n%s", on)
	}
}
