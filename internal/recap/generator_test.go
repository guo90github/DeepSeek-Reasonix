package recap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	last    string
	// failures makes the next N calls fail the way a provider refuses an
	// oversized request, which is what the lane's smaller-retry answers.
	failures int
}

// lastEvidence is what the lane actually asked with: the budget failure this
// guards is about the request, not the answer.
// lastMessageText is the request's last user-role message: what the lane actually
// asked with.
func lastMessageText(req provider.Request) string {
	for _, v := range slices.Backward(req.Messages) {
		if v.Role == provider.RoleUser {
			return v.Content
		}
	}
	return ""
}

func (p *fakeProvider) lastEvidence() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
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

func (p *fakeProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	if p.failures > 0 {
		p.failures--
		p.calls++
		p.mu.Unlock()
		return nil, fmt.Errorf("prompt is too long: %d bytes", len(lastMessageText(req)))
	}
	p.active++
	p.calls++
	p.last = lastMessageText(req)
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
	if res.Stored || res.Reason != "empty answer" {
		t.Fatalf("two empty answers must leave the session pending and say why: %+v", res)
	}
	if pending, err := h.store.PendingMap(ctx); err != nil || pending[path].Reason != "empty answer twice: the model returned nothing" {
		t.Fatalf("the reason must name an empty answer instead of a parse failure: %+v (%v)", pending, err)
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
		{"kind":"fact","body":"the admin call to http://10.0.0.5:8080/admin failed","evidence":"curl http://10.0.0.5:8080/admin",
		 "refs":[{"kind":"command","value":"curl http://10.0.0.5:8080/admin","detail":"from http://10.0.0.5:8080"}],
		 "scope":{"level":"project","reason":"only true for http://10.0.0.5:8080"}}
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
	// The pointers and the tier proposal are what make a note checkable: dropping
	// them on the way in would leave the page and the next session with nothing to
	// point at, which is exactly what v8's prompt asks for by name.
	if len(entry.Refs) != 1 || entry.Refs[0].Kind != "command" {
		t.Fatalf("the note's pointer was dropped: %+v", entry)
	}
	if strings.Contains(entry.Refs[0].Value, "10.0.0.5") || strings.Contains(entry.Refs[0].Detail, "10.0.0.5") {
		t.Fatalf("internal address survived redaction in a pointer: %+v", entry.Refs)
	}
	if entry.Scope.Level != "project" || strings.Contains(entry.Scope.Reason, "10.0.0.5") {
		t.Fatalf("the tier proposal was dropped or left unredacted: %+v", entry.Scope)
	}
}

func TestGenerateSingleFlight(t *testing.T) {
	ctx := context.Background()
	prov := &fakeProvider{name: "fake", answer: goodAnswer, delay: 40 * time.Millisecond}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = fakeModels{prov: prov, ref: "fake/model", ok: true}
	})
	var wg sync.WaitGroup
	for i := range 4 {
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
	got, ok := parseEntries("```json\n[{\"kind\":\"fact\",\"body\":\"b1\",\"evidence\":\"e1\"," +
		"\"refs\":[{\"kind\":\"path\",\"value\":\"desktop/x.go\",\"detail\":\"L40\"},{\"value\":\"go test ./internal/recap/\"}]," +
		"\"scope\":{\"level\":\"base\",\"reason\":\"true on this machine\"}}," +
		"{\"kind\":\"follow-up\",\"body\":\"b2\"}]\n```")
	if !ok || len(got) != 2 {
		t.Fatalf("parse = %+v ok=%v", got, ok)
	}
	if got[0].Kind != KindFact || got[0].Evidence != "e1" || got[1].Kind != KindHandoff {
		t.Fatalf("entries = %+v", got)
	}
	if len(got[0].Refs) != 2 || got[0].Refs[0].Kind != RefPath || got[0].Refs[0].Detail != "L40" {
		t.Fatalf("a pointer must keep its kind and detail: %+v", got[0].Refs)
	}
	// A missing kind is inferred from the value: the pointer is the part worth
	// having, and a command is recognizable without a label.
	if got[0].Refs[1].Kind != RefCommand {
		t.Fatalf("a labelled-nowhere command must be read as a command: %+v", got[0].Refs[1])
	}
	if got[0].Scope.Level != ScopeBase || got[0].Scope.Reason == "" {
		t.Fatalf("a proposed tier must survive with its reason: %+v", got[0].Scope)
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

// A tier or a pointer the rule does not know must not become a claim: the note
// stays in its project and the unusable pointer is dropped, while the rest of the
// note survives.
func TestParseEntriesKeepsOnlyUsablePointersAndTiers(t *testing.T) {
	got, ok := parseEntries(`[{"kind":"fact","body":"kept",
		"refs":[{"kind":"vibes","value":"   "},{"kind":"vibes","value":"1-2"}],
		"scope":{"level":"everything","reason":"why not"}}]`)
	if !ok || len(got) != 1 {
		t.Fatalf("parse = %+v ok=%v", got, ok)
	}
	if len(got[0].Refs) != 1 || got[0].Refs[0].Kind != RefTurn {
		t.Fatalf("an unlabelled turn range must survive as a turn, the empty one must not: %+v", got[0].Refs)
	}
	if got[0].Scope.Level != "" {
		t.Fatalf("a tier outside the three must be dropped, got %q", got[0].Scope.Level)
	}
	if MemoryScopeFor("everything") != "project" || MemoryScopeFor(ScopeGeneric) != "global" {
		t.Fatal("an unknown tier lands where unproposed notes land; a generic one would go to the person")
	}
}

func TestParseEntriesDropsUnknownKindsAndCapsTheList(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[{"kind":"fact","body":"kept"},{"kind":"gibberish","body":"dropped"},{"body":"kindless"},`)
	for i := range maxEntries + 4 {
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
	if len(got) > 400 {
		t.Fatalf("clip kept %d bytes, want the budget or less", len(got))
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

// The dropped middle is where conclusions get drawn in passing, so an over-long
// session must still show what each of those turns was about rather than only
// how many bytes were dropped.
func TestClipForRecapDigestsTheOmittedTurns(t *testing.T) {
	build := func(turns int) string {
		var b strings.Builder
		b.WriteString(strings.Repeat("filler\n", 300))
		for turn := 1; turn <= turns; turn++ {
			fmt.Fprintf(&b, "## User (turn %d)\nasked for thing %d\n%s\n", turn, turn, strings.Repeat("body\n", 30))
		}
		b.WriteString(strings.Repeat("filler\n", 300))
		return b.String()
	}
	// Roomy: every dropped turn is mapped by its own first line.
	got := clipForRecap(build(6), 3000)
	for turn := 1; turn <= 6; turn++ {
		if !strings.Contains(got, fmt.Sprintf("## User (turn %d) asked for thing %d", turn, turn)) {
			t.Fatalf("turn %d vanished from the digest: %q", turn, got)
		}
	}
	if !strings.Contains(got, "those turns came down to") || !strings.Contains(got, "end of omitted middle") {
		t.Fatalf("the digest is not marked as an omission: %q", got)
	}
	if strings.Contains(got, strings.Repeat("body\n", 3)) {
		t.Fatalf("the digest copied a turn's body instead of one line: %q", got)
	}
	if len(got) > 3000 {
		t.Fatalf("the digest blew the input budget: %d bytes", len(got))
	}
	// Tight: the map stays short and says how many turns it left out.
	tight := clipForRecap(build(40), 1000)
	if !strings.Contains(tight, "more turns)") {
		t.Fatalf("a short digest must count the turns it left out: %q", tight)
	}
	if len(tight) > 1000 {
		t.Fatalf("a tight clip blew the input budget: %d bytes", len(tight))
	}
}

// The lane yields while a session turn is in flight instead of competing with the
// conversation it belongs to — but only for so long: a lane that yields forever
// starves every later recap behind one busy session. Both halves are the point.
func TestTheLaneYieldsToARunningSessionAndGivesUpAtItsBudget(t *testing.T) {
	ctx := context.Background()
	busy := true
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Busy = func() bool { return busy }
		o.YieldInterval = time.Millisecond
		o.YieldBudget = 5 * time.Millisecond
	})
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatalf("a busy session must leave the recap pending, not fail the lane: %v", err)
	}
	if !res.Skipped || res.Reason != "session busy" {
		t.Fatalf("result = %+v, want a skip that says why", res)
	}
	pending, err := h.store.PendingMap(ctx)
	if err != nil {
		t.Fatalf("pending map: %v", err)
	}
	// A pending marker is what a later sweep retries; without one the yield is
	// indistinguishable from silence.
	if entry, ok := pending[path]; !ok || entry.Attempts == 0 || !strings.Contains(entry.Reason, "session busy") {
		t.Fatalf("the yielded recap must be marked pending with its reason: %+v", pending)
	}

	// The same lane runs the moment the session is idle again.
	busy = false
	res, err = h.generator.Generate(ctx, path)
	if err != nil || !res.Stored {
		t.Fatalf("an idle session must let the lane through: %+v (%v)", res, err)
	}
}

// A long session fills the transcript budget right up to its cap, so whatever the
// request spends on its own policy has to be budgeted for. The failure this guards
// was "bounded reviewer request exceeds 114688 bytes" on the longest sessions.
func TestGenerateHandlesASessionThatFillsTheTranscriptBudget(t *testing.T) {
	ctx := context.Background()
	// CJK, because that is what this user's sessions look like and a rune is three
	// bytes: a budget counted in either one must still fit.
	line := "这一轮我在查打包脚本里的路径处理，顺便核对了 NSIS 与 portable 目录的约定。\n"
	// 96 KiB is the lane's default transcript budget; the session below is far past
	// it, so the clip fills the budget and the request's own policy has to fit too.
	const transcriptBudget = 96 * 1024
	// The markers are what the middle digest is built from, and the digest is what the
	// clip used to spend on top of its budget: a fixture without them never reached the
	// overflow this test exists for.
	turn := "## User (turn %d)\n" + line + "\n## Assistant (turn %d)\n我先读了 scripts/desktop-build.sh。\n\n"
	var b strings.Builder
	for turnNumber := 1; b.Len() <= transcriptBudget*2; turnNumber++ {
		fmt.Fprintf(&b, turn, turnNumber, turnNumber)
	}
	body := b.String()
	h := newHarness(t, func(o *GeneratorOptions) { o.Transcript = fakeTranscript{text: body} })
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", body)

	res, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !res.Stored {
		t.Fatalf("a long session must still be recapped: %+v", res)
	}
	asked := h.provider.lastEvidence()
	if len(asked) > transcriptBudget {
		t.Fatalf("evidence sent = %d bytes, want the transcript budget or less", len(asked))
	}
	if len(asked) == 0 {
		t.Fatal("the lane must send evidence")
	}
}
