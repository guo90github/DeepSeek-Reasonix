package recap

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type splitTranscript struct {
	fast          string
	authoritative string
}

func (s splitTranscript) Read(context.Context, string) (string, error) { return s.fast, nil }

func (s splitTranscript) ReadAuthoritative(context.Context, string) (string, error) {
	return s.authoritative, nil
}

type requestRecordingProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *requestRecordingProvider) Name() string { return "recap-shadow-test" }

func (p *requestRecordingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "Goal: g\nActions: a\nConclusion: c\nFollow-ups: none\n"}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	close(ch)
	return ch, nil
}

func (p *requestRecordingProvider) userPrompts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.reqs))
	for _, req := range p.reqs {
		for _, m := range req.Messages {
			if m.Role == provider.RoleUser {
				out = append(out, m.Content)
			}
		}
	}
	return out
}

type fixedModelResolver struct{ prov provider.Provider }

func (r fixedModelResolver) Resolve(context.Context, string) (provider.Provider, string, bool) {
	return r.prov, "test-model", true
}

// The first fast reads of a process are checked against the replay: a mismatch
// must reach the model as the authoritative text, must be recorded, and the
// budget must stop paying for the replay on later closes.
func TestShadowCheckReplacesAMismatchedFastRead(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	first := savedSession(t, dir)
	second := savedSession(t, filepath.Join(dir, "b"))

	store, err := Open(ctx, Options{InMemory: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()

	recorder := &requestRecordingProvider{}
	transcript := splitTranscript{
		fast:          "## User (turn 1)\nfast text\n\n",
		authoritative: "## User (turn 1)\nauthoritative text\n\n",
	}
	generator := NewGenerator(GeneratorOptions{
		Store:        store,
		Models:       fixedModelResolver{prov: recorder},
		Transcript:   transcript,
		Sink:         event.Discard,
		ShadowChecks: 1,
	})

	for _, path := range []string{first, second} {
		if _, err := generator.Generate(ctx, path); err != nil {
			t.Fatalf("generate %s: %v", path, err)
		}
	}
	prompts := recorder.userPrompts()
	if len(prompts) != 2 {
		t.Fatalf("model calls = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[0], "authoritative text") {
		t.Fatalf("the shadow check did not take the replay's text: %q", prompts[0])
	}
	if !strings.Contains(prompts[1], "fast text") {
		t.Fatalf("the budget should stop verifying after one check: %q", prompts[1])
	}

	entries, err := store.Activity(ctx, 10)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	checks := 0
	for _, entry := range entries {
		if entry.Stage != "fastpath" {
			continue
		}
		checks++
		if !strings.Contains(entry.Detail, "mismatch") {
			t.Fatalf("fast-path decision recorded as %q", entry.Detail)
		}
	}
	if checks != 1 {
		t.Fatalf("fastpath decisions = %d, want exactly one", checks)
	}
}
