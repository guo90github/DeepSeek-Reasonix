package recap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type fakeSink struct {
	mu     sync.Mutex
	events []event.Event
}

func (s *fakeSink) Emit(e event.Event) {
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
}

func (s *fakeSink) usageSources() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.events))
	for _, e := range s.events {
		out = append(out, e.UsageSource)
	}
	return out
}

type fakeProvider struct {
	name    string
	answer  string
	delay   time.Duration
	mu      sync.Mutex
	active  int
	maxSeen int
	calls   int
}

func (p *fakeProvider) Name() string { return p.name }

func (p *fakeProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.active++
	p.calls++
	if p.active > p.maxSeen {
		p.maxSeen = p.active
	}
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 2)
	go func() {
		defer close(ch)
		defer func() {
			p.mu.Lock()
			p.active--
			p.mu.Unlock()
		}()
		if p.delay > 0 {
			select {
			case <-time.After(p.delay):
			case <-ctx.Done():
				return
			}
		}
		ch <- provider.Chunk{Type: provider.ChunkText, Text: p.answer}
		ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	}()
	return ch, nil
}

func (p *fakeProvider) counters() (calls, maxSeen int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, p.maxSeen
}

type fakeModels struct {
	prov provider.Provider
	ref  string
	ok   bool
}

func (m fakeModels) Resolve(context.Context, string) (provider.Provider, string, bool) {
	return m.prov, m.ref, m.ok
}

type fakeTranscript struct{ text string }

func (r fakeTranscript) Read(context.Context, string) (string, error) { return r.text, nil }

const goodAnswer = "Goal: make the parser accept the new syntax\n" +
	"Actions: read the grammar, patched it, ran the tests\n" +
	"Conclusion: green and verified with go test\n" +
	"Follow-ups: none\n"

type harness struct {
	generator *Generator
	store     *Store
	sink      *fakeSink
	provider  *fakeProvider
	dir       string
}

func newHarness(t *testing.T, opts ...func(*GeneratorOptions)) *harness {
	t.Helper()
	dir := t.TempDir()
	prov := &fakeProvider{name: "fake", answer: goodAnswer}
	sink := &fakeSink{}
	store := openTestStore(t)
	gopts := GeneratorOptions{
		Store:      store,
		Models:     fakeModels{prov: prov, ref: "fake/model", ok: true},
		Transcript: fakeTranscript{text: "user asked something\nassistant answered\n"},
		Sink:       sink,
	}
	for _, apply := range opts {
		apply(&gopts)
	}
	return &harness{generator: NewGenerator(gopts), store: store, sink: sink, provider: prov, dir: dir}
}

func (h *harness) session(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(h.dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGenerateStoresAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\ntwo\n")

	first, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !first.Stored || first.Record.Goal == "" {
		t.Fatalf("first generate did not store a recap: %+v", first)
	}
	if first.Record.PromptVersion != PromptVersion || first.Record.Model != "fake/model" {
		t.Fatalf("record provenance missing: %+v", first.Record)
	}

	second, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if second.Stored || second.Reason != "current" {
		t.Fatalf("unchanged session should be left alone: %+v", second)
	}
	if calls, _ := h.provider.counters(); calls != 1 {
		t.Fatalf("provider called %d times for an unchanged session, want 1", calls)
	}
}

func TestGenerateRecomputesAfterRewrite(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	if _, err := h.generator.Generate(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("one\ntwo\nthree much longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Stored {
		t.Fatalf("a rewritten session must be recapped again: %+v", again)
	}
	if calls, _ := h.provider.counters(); calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
}

func TestGenerateEmitsItsOwnUsageSource(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	if _, err := h.generator.Generate(ctx, path); err != nil {
		t.Fatal(err)
	}
	sources := h.sink.usageSources()
	if len(sources) != 1 || sources[0] != event.UsageSourceSessionRecap {
		t.Fatalf("usage sources = %v, want [%s]", sources, event.UsageSourceSessionRecap)
	}
}

func TestGenerateRedactsInternalAddresses(t *testing.T) {
	ctx := context.Background()
	prov := &fakeProvider{name: "fake", answer: "Goal: call the service\n" +
		"Actions: curl http://10.0.0.5:8080/admin failed\n" +
		"Conclusion: fixed\nFollow-ups: none\n"}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	h.provider = prov
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stored {
		t.Fatalf("expected a stored recap: %+v", res)
	}
	if strings.Contains(res.Record.Actions, "10.0.0.5") {
		t.Fatalf("internal address survived redaction: %q", res.Record.Actions)
	}
	if !strings.Contains(res.Record.Actions, "[redacted]") {
		t.Fatalf("expected the redaction placeholder: %q", res.Record.Actions)
	}
}

func TestGenerateSingleFlight(t *testing.T) {
	ctx := context.Background()
	prov := &fakeProvider{name: "fake", answer: goodAnswer, delay: 40 * time.Millisecond}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		path := h.session(t, "2026010"+string(rune('1'+i))+"-000000.000000000-fake.jsonl", "one\n")
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if _, err := h.generator.Generate(ctx, p); err != nil {
				t.Errorf("generate: %v", err)
			}
		}(path)
	}
	wg.Wait()
	if _, maxSeen := prov.counters(); maxSeen != 1 {
		t.Fatalf("recap lane ran %d calls at once, want 1", maxSeen)
	}
}

func TestGenerateMarksPendingWithoutModel(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, func(o *GeneratorOptions) { o.Models = fakeModels{ok: false} })
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored || !res.Skipped {
		t.Fatalf("expected a skip without a model: %+v", res)
	}
	records, pending, err := h.store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if records != 0 || pending != 1 {
		t.Fatalf("counts = (%d,%d), want (0,1)", records, pending)
	}
	if calls, _ := h.provider.counters(); calls != 0 {
		t.Fatalf("provider must not be called without a model, calls=%d", calls)
	}
}

func TestGenerateMarksPendingOnUnparseableAnswer(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: &fakeProvider{name: "fake", answer: "sorry, no idea"}, ref: "fake/model", ok: true}
	})
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored || res.Reason != "unparseable answer" {
		t.Fatalf("expected an unparseable-answer skip: %+v", res)
	}
	if _, pending, _ := h.store.Counts(ctx); pending != 1 {
		t.Fatalf("unparseable answer must leave a pending marker, got %d", pending)
	}
}

func TestGenerateSkipsInadmissible(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	res, err := h.generator.Generate(ctx, filepath.Join(h.dir, "20260101-000000.000000000-fake-recovery-0123456789abcdef.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored || !res.Skipped {
		t.Fatalf("recovery copy must be skipped: %+v", res)
	}
	if calls, _ := h.provider.counters(); calls != 0 {
		t.Fatalf("provider must not be called for an inadmissible session, calls=%d", calls)
	}
}

func TestParseElements(t *testing.T) {
	got, ok := parseElements("Goal: g\nActions: a\nConclusion: c\nFollow-ups: f\n")
	if !ok || got.Goal != "g" || got.Actions != "a" || got.Conclusion != "c" || got.FollowUps != "f" {
		t.Fatalf("parse = %+v ok=%v", got, ok)
	}
	if _, ok := parseElements("Goal: only one line"); ok {
		t.Fatal("a partial answer must not parse")
	}
}

func TestClipForRecapKeepsHeadAndTail(t *testing.T) {
	head := strings.Repeat("head line\n", 40)
	tail := strings.Repeat("tail line\n", 40)
	text := head + strings.Repeat("middle line\n", 200) + tail
	got := clipForRecap(text, 400)
	if len(got) > 400+64 {
		t.Fatalf("clip kept %d bytes, want near 400", len(got))
	}
	if !strings.HasPrefix(got, "head line") || !strings.HasSuffix(got, "tail line\n") {
		t.Fatalf("clip dropped an end of the transcript: %q", got)
	}
	if !strings.Contains(got, "bytes omitted") {
		t.Fatalf("clip did not mark the omission: %q", got)
	}
	if short := clipForRecap("tiny", 400); short != "tiny" {
		t.Fatalf("short transcript must pass through, got %q", short)
	}
}
