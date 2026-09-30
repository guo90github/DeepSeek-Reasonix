package recap

import (
	"context"
	"fmt"
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

// emptyFirstProvider returns empty leading answers (the zero value means one),
// then a usable array: this is what a reasoning model that spends its whole
// completion budget on thinking looks like.
type emptyFirstProvider struct {
	mu    sync.Mutex
	empty int
	calls int
}

func (p *emptyFirstProvider) Name() string { return "fake" }

func (p *emptyFirstProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	text := goodAnswer
	if call <= max(p.empty, 1) {
		text = ""
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: text}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	close(ch)
	return ch, nil
}

func (p *emptyFirstProvider) callsMade() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
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

const goodAnswer = `[
  {"kind":"fact","body":"the parser accepts the new syntax","evidence":"internal/parser.go"},
  {"kind":"handoff","body":"the grammar docs still describe the old syntax"}
]`

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
	if !first.Stored || len(first.Record.Entries) != 2 {
		t.Fatalf("first generate did not store two notes: %+v", first)
	}
	if first.Record.Entries[0].Kind != KindFact || first.Record.Entries[1].Kind != KindHandoff {
		t.Fatalf("kinds not preserved: %+v", first.Record.Entries)
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

func TestGenerateStoresAnEmptyAnswerOnce(t *testing.T) {
	ctx := context.Background()
	prov := &fakeProvider{name: "fake", answer: "[]"}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	first, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Stored || len(first.Record.Entries) != 0 {
		t.Fatalf("an empty answer must be stored as a recap without notes: %+v", first)
	}
	second, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Skipped || second.Reason != "current" {
		t.Fatalf("an empty recap must not be recomputed: %+v", second)
	}
	if calls, _ := prov.counters(); calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

func TestGenerateRetriesAnEmptyAnswer(t *testing.T) {
	ctx := context.Background()
	prov := &emptyFirstProvider{}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stored || len(res.Record.Entries) != 2 {
		t.Fatalf("the retry's answer must be stored: %+v", res)
	}
	if calls := prov.callsMade(); calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (one retry)", calls)
	}
}

func TestGenerateGivesUpAfterASecondEmptyAnswer(t *testing.T) {
	ctx := context.Background()
	prov := &emptyFirstProvider{empty: 2}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored || res.Reason != "unparseable answer" {
		t.Fatalf("two empty answers must leave the session pending: %+v", res)
	}
	if calls := prov.callsMade(); calls != 2 {
		t.Fatalf("provider calls = %d, want 2 and no more", calls)
	}
	if _, pending, _ := h.store.Counts(ctx); pending != 1 {
		t.Fatalf("pending = %d, want 1", pending)
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
	prov := &fakeProvider{name: "fake", answer: `[
		{"kind":"fact","body":"the admin call to http://10.0.0.5:8080/admin failed","evidence":"curl http://10.0.0.5:8080/admin"}
	]`}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	h.provider = prov
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")
	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stored || len(res.Record.Entries) != 1 {
		t.Fatalf("expected one stored note: %+v", res)
	}
	entry := res.Record.Entries[0]
	if strings.Contains(entry.Body, "10.0.0.5") || strings.Contains(entry.Evidence, "10.0.0.5") {
		t.Fatalf("internal address survived redaction: %+v", entry)
	}
	if !strings.Contains(entry.Body, "[redacted]") {
		t.Fatalf("expected the redaction placeholder: %+v", entry)
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

func TestParseEntries(t *testing.T) {
	got, ok := parseEntries("```json\n[{\"kind\":\"fact\",\"body\":\"b1\",\"evidence\":\"e1\"}," +
		"{\"kind\":\"follow-up\",\"body\":\"b2\"}]\n```")
	if !ok || len(got) != 2 {
		t.Fatalf("parse = %+v ok=%v", got, ok)
	}
	if got[0].Kind != KindFact || got[0].Evidence != "e1" || got[1].Kind != KindHandoff {
		t.Fatalf("entries = %+v", got)
	}
	if empty, ok := parseEntries("[]"); !ok || len(empty) != 0 {
		t.Fatalf("an empty array is a valid answer: %+v ok=%v", empty, ok)
	}
	if _, ok := parseEntries("sorry, no idea"); ok {
		t.Fatal("an answer carrying no array must not parse")
	}
	if _, ok := parseEntries(`[{"kind":"fact","body"`); ok {
		t.Fatal("truncated JSON must not parse")
	}
}

func TestParseEntriesDropsUnknownKindsAndCapsTheList(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[{"kind":"fact","body":"kept"},{"kind":"gibberish","body":"dropped"},{"body":"kindless"},`)
	for i := 0; i < maxEntries+4; i++ {
		fmt.Fprintf(&b, `{"kind":"refuted","body":"r%d"},`, i)
	}
	b.WriteString(`{"kind":"refuted","body":"r0"}]`)
	got, ok := parseEntries(b.String())
	if !ok {
		t.Fatal("a mixed answer must still parse")
	}
	if len(got) != maxEntries {
		t.Fatalf("got %d entries, want the cap %d", len(got), maxEntries)
	}
	if got[0].Kind != KindFact || got[0].Body != "kept" {
		t.Fatalf("the first valid note was lost: %+v", got[0])
	}
}

func TestParseEntriesSalvagesATruncatedAnswer(t *testing.T) {
	got, ok := parseEntries(`[{"kind":"fact","body":"kept","evidence":"a.go"},` +
		`{"kind":"handoff","body":"cut off mid-`)
	if !ok || len(got) != 1 || got[0].Body != "kept" || got[0].Evidence != "a.go" {
		t.Fatalf("a truncated answer must keep the notes it finished: %+v ok=%v", got, ok)
	}
}

func TestParseEntriesKeepsBracesInsideAValue(t *testing.T) {
	got, ok := parseEntries(`[{"kind":"fact","body":"emits {\"a\":1} on start"},{"kind":"refuted","body":"cu`)
	if !ok || len(got) != 1 || !strings.Contains(got[0].Body, `{"a":1}`) {
		t.Fatalf("a brace inside a string must not end the object: %+v ok=%v", got, ok)
	}
}

func TestAnswerExcerptRedactsAndClips(t *testing.T) {
	excerpt := answerExcerpt("prose reply\nsee http://10.0.0.5:8080 and " + strings.Repeat("字", 300))
	if strings.Contains(excerpt, "10.0.0.5") || !strings.Contains(excerpt, "[redacted]") {
		t.Fatalf("the excerpt kept an internal address: %q", excerpt)
	}
	if strings.Contains(excerpt, "\n") {
		t.Fatalf("the excerpt kept a newline: %q", excerpt)
	}
	if runes := []rune(excerpt); len(runes) > 121 {
		t.Fatalf("the excerpt ran to %d runes", len(runes))
	}
}

func TestSinkFollowsTheKind(t *testing.T) {
	if Sink(KindHandoff) != SinkDisplay {
		t.Fatalf("a handoff is shown but not stored, got %q", Sink(KindHandoff))
	}
	for _, kind := range []string{KindFact, KindRootCause, KindRefuted} {
		if Sink(kind) != SinkMemory {
			t.Fatalf("%s must land in memory, got %q", kind, Sink(kind))
		}
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
