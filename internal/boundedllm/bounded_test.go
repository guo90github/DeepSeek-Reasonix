package boundedllm

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

// A negative budget means "no cap of ours" — the one escape hatch a long-session
// caller needs, because the model's own output budget is the only ceiling that
// knows what the model can do. Zero still means "use the default".
type recordingProvider struct {
	requests []provider.Request
	text     string
}

func (p *recordingProvider) Name() string { return "recorder" }

func (p *recordingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.requests = append(p.requests, req)
	ch := make(chan provider.Chunk, 3)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: p.text}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	close(ch)
	return ch, nil
}

func TestNegativeBudgetsRemoveTheCapsOfOursByHand(t *testing.T) {
	ctx := context.Background()
	long := strings.Repeat("x", DefaultMaxOutputBytes+1024)

	uncapped := &recordingProvider{text: long}
	if _, err := Call(ctx, Config{Provider: uncapped, MaxTokens: -1, MaxOutputBytes: -1},
		"system", "evidence"); err != nil {
		t.Fatalf("an uncapped call must be allowed to answer at length: %v", err)
	}
	if got := uncapped.requests[0].MaxTokens; got != 0 {
		t.Fatalf("MaxTokens forwarded = %d, want 0 (the model's own ceiling)", got)
	}

	capped := &recordingProvider{text: long}
	if _, err := Call(ctx, Config{Provider: capped}, "system", "evidence"); err == nil {
		t.Fatal("the default byte cap must still abort a runaway answer")
	}
	if got := capped.requests[0].MaxTokens; got != DefaultMaxTokens {
		t.Fatalf("MaxTokens forwarded = %d, want the default %d", got, DefaultMaxTokens)
	}
}
