package recap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/boundedllm"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/secrets"
)

// Elements is the parsed four-element body of one recap.
type Elements struct {
	Goal       string
	Actions    string
	Conclusion string
	FollowUps  string
}

const recapSystemPrompt = `You write a short retrospective of one finished coding session for the person who ran it.
Answer with exactly four lines, in this order, and nothing else:
Goal: what the session set out to do, one sentence
Actions: the key steps taken, in order, at most two sentences
Conclusion: what was achieved or learned, and how it was verified
Follow-ups: what is still unfinished, or "none"

Report the session's final state, not what it merely discussed:
- Names, identifiers, file paths, decision numbers, version strings and field
  values are copied verbatim from the transcript. Never paraphrase, translate,
  shorten or round them (a range stays a range: "O1-O9" is not "O1-O6").
- An option that was raised and rejected is not a decision. Report what the
  session settled on; if the transcript does not show a settled answer, leave
  that detail out instead of guessing.
- The transcript may be trimmed, so never invent detail to fill a gap.
Never include secrets, credentials, hostnames, or internal addresses.`

// ModelResolver returns the provider used for one recap: the model the session
// itself recorded, then the configured fallback. ok=false marks the session
// pending instead of guessing a model.
type ModelResolver interface {
	Resolve(ctx context.Context, sessionPath string) (provider.Provider, string, bool)
}

// TranscriptReader returns the authoritative text of one session.
type TranscriptReader interface {
	Read(ctx context.Context, sessionPath string) (string, error)
}

// ShadowTranscript is implemented by readers that can also render through the
// authoritative path, so the lane can verify a fast read on a real machine.
type ShadowTranscript interface {
	ReadAuthoritative(ctx context.Context, sessionPath string) (string, error)
}

// GeneratorOptions wires the recap lane. Sink, Models, and Transcript are
// supplied by the composition root so this package never reaches a controller.
type GeneratorOptions struct {
	// Store is an already-open projection; StoreFactory opens one per attempt
	// instead. A long-lived handle keeps the projection file locked, which a
	// disposable projection must not do.
	Store        *Store
	StoreFactory func(context.Context) (*Store, error)
	Models       ModelResolver
	Transcript   TranscriptReader
	Sink         event.Sink
	Removing     Remover
	Now          func() time.Time
	// Busy reports whether a session turn is in flight. The recap lane yields
	// instead of competing with it, but only for YieldBudget: a lane that yields
	// forever would starve every later recap behind one busy session.
	Busy          func() bool
	Timeout       time.Duration
	MaxTokens     int
	MaxInputBytes int
	YieldInterval time.Duration
	YieldBudget   time.Duration
	// ShadowChecks verifies the first N fast reads of a process against the
	// authoritative render; 0 keeps the default and a negative value disables it.
	ShadowChecks int
}

// defaultShadowChecks bounds in-the-field verification of the fast read: enough
// to prove it on a real machine, far from every close.
const defaultShadowChecks = 2

// Generator produces recaps on a lane of its own: one call at a time, its own
// usage source, and no session context.
type Generator struct {
	opts         GeneratorOptions
	gate         chan struct{}
	shadowBudget chan struct{}
}

// NewGenerator builds the lane. The gate is what keeps recaps from running in
// parallel with each other.
func NewGenerator(opts GeneratorOptions) *Generator {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 90 * time.Second
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 700
	}
	if opts.MaxInputBytes <= 0 {
		opts.MaxInputBytes = 96 * 1024
	}
	if opts.YieldInterval <= 0 {
		opts.YieldInterval = 250 * time.Millisecond
	}
	if opts.YieldBudget <= 0 {
		opts.YieldBudget = 5 * time.Second
	}
	if opts.ShadowChecks == 0 {
		opts.ShadowChecks = defaultShadowChecks
	}
	budget := opts.ShadowChecks
	if budget < 0 {
		budget = 0
	}
	shadowBudget := make(chan struct{}, budget)
	for i := 0; i < budget; i++ {
		shadowBudget <- struct{}{}
	}
	return &Generator{opts: opts, gate: make(chan struct{}, 1), shadowBudget: shadowBudget}
}

// Result reports what one Generate attempt did.
type Result struct {
	Record  Record
	Stored  bool
	Skipped bool
	Reason  string
}

// Generate writes at most one recap for a session. It is idempotent: an
// unchanged fingerprint with the current prompt version is left alone.
func (g *Generator) Generate(ctx context.Context, sessionPath string) (Result, error) {
	path := strings.TrimSpace(sessionPath)
	result, err := g.generate(ctx, path)
	g.trace(ctx, nil, "generate", path, describeResult(result, err))
	return result, err
}

// describeResult renders one attempt's outcome for the decision log.
func describeResult(result Result, err error) string {
	switch {
	case err != nil:
		return "error: " + err.Error()
	case result.Stored:
		return "stored"
	case result.Skipped:
		return "skip: " + result.Reason
	default:
		return "no-op"
	}
}

func (g *Generator) generate(ctx context.Context, path string) (Result, error) {
	if !Admissible(path, g.opts.Removing) {
		return Result{Skipped: true, Reason: "inadmissible"}, nil
	}
	fingerprint, err := Fingerprint(path)
	if err != nil {
		return Result{Skipped: true, Reason: err.Error()}, nil
	}
	store, release, err := g.storeFor(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()
	if current, ok, err := store.Get(ctx, path); err != nil {
		return Result{}, err
	} else if ok && current.Fingerprint == fingerprint && current.PromptVersion == PromptVersion {
		return Result{Skipped: true, Reason: "current"}, nil
	}
	if err := g.acquire(ctx); err != nil {
		if errors.Is(err, errLaneBusy) {
			_ = store.MarkPending(ctx, path, "session busy", g.opts.Now())
			return Result{Skipped: true, Reason: "session busy"}, nil
		}
		return Result{}, err
	}
	defer func() { <-g.gate }()

	text, err := readCovered(ctx, g, store, path)
	if err != nil {
		_ = store.MarkPending(ctx, path, "transcript unreadable: "+err.Error(), g.opts.Now())
		return Result{Skipped: true, Reason: err.Error()}, nil
	}
	text = verifyFastPath(ctx, g, store, path, text)
	text = clipForRecap(text, g.opts.MaxInputBytes)
	if strings.TrimSpace(text) == "" {
		return Result{Skipped: true, Reason: "empty transcript"}, nil
	}
	prov, ref, ok := g.opts.Models.Resolve(ctx, path)
	if !ok {
		_ = store.MarkPending(ctx, path, "no usable model", g.opts.Now())
		return Result{Skipped: true, Reason: "no usable model"}, nil
	}
	raw, err := boundedllm.Call(ctx, boundedllm.Config{
		Provider:       prov,
		ModelRef:       ref,
		Sink:           g.opts.Sink,
		UsageSource:    event.UsageSourceSessionRecap,
		Timeout:        g.opts.Timeout,
		MaxTokens:      g.opts.MaxTokens,
		MaxOutputBytes: 8 * 1024,
		MaxSystemBytes: 4 * 1024,
		MaxTotalBytes:  g.opts.MaxInputBytes + 4*1024,
		EffortOverride: provider.PreferredReasoning(prov, "low"),
	}, recapSystemPrompt, text)
	if err != nil {
		_ = store.MarkPending(ctx, path, err.Error(), g.opts.Now())
		return Result{Skipped: true, Reason: "call failed"}, nil
	}
	elements, ok := parseElements(raw)
	if !ok {
		_ = store.MarkPending(ctx, path, "unparseable answer", g.opts.Now())
		return Result{Skipped: true, Reason: "unparseable answer"}, nil
	}
	rec := Record{
		Path:          path,
		Fingerprint:   fingerprint,
		Goal:          secrets.Redact(elements.Goal),
		Actions:       secrets.Redact(elements.Actions),
		Conclusion:    secrets.Redact(elements.Conclusion),
		FollowUps:     secrets.Redact(elements.FollowUps),
		Model:         ref,
		PromptVersion: PromptVersion,
		GeneratedAt:   g.opts.Now(),
	}
	if err := store.Put(ctx, rec); err != nil {
		return Result{}, err
	}
	return Result{Record: rec, Stored: true}, nil
}

// storeFor resolves the projection for one attempt. A factory opens and closes
// per call so no file handle outlives the work.
func (g *Generator) storeFor(ctx context.Context) (*Store, func(), error) {
	if g.opts.StoreFactory != nil {
		store, err := g.opts.StoreFactory(ctx)
		if err != nil {
			return nil, func() {}, err
		}
		return store, func() { _ = store.Close() }, nil
	}
	return g.opts.Store, func() {}, nil
}

// markPending records a deferred session without holding a store open.
func (g *Generator) markPending(ctx context.Context, path, reason string) {
	store, release, err := g.storeFor(ctx)
	if err != nil {
		return
	}
	defer release()
	_ = store.MarkPending(ctx, path, reason, g.opts.Now())
}

// trace records one lane decision, reusing an open store when the caller has one
// so the common path pays no second open.
func (g *Generator) trace(ctx context.Context, store *Store, stage, path, detail string) {
	if store == nil {
		opened, release, err := g.storeFor(ctx)
		if err != nil {
			return
		}
		defer release()
		store = opened
	}
	_ = store.Trace(ctx, stage, path, detail, g.opts.Now())
}

// errLaneBusy reports that a running session held the lane past its budget.
var errLaneBusy = errors.New("recap: session busy")

// acquire takes the single lane slot, waiting while a session turn is in flight
// so a recap never competes with the conversation it belongs to. Waiting is
// bounded: past YieldBudget the session is left pending for a later sweep.
func (g *Generator) acquire(ctx context.Context) error {
	deadline := g.opts.Now().Add(g.opts.YieldBudget)
	for {
		select {
		case g.gate <- struct{}{}:
			if g.opts.Busy == nil || !g.opts.Busy() {
				return nil
			}
			<-g.gate
		case <-ctx.Done():
			return ctx.Err()
		}
		if g.opts.Now().After(deadline) {
			return errLaneBusy
		}
		select {
		case <-time.After(g.opts.YieldInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// parseElements reads the four labelled lines. An answer missing the first
// three labels is rejected rather than stored half-formed.
func parseElements(raw string) (Elements, bool) {
	var out Elements
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		label, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "goal":
			out.Goal = value
		case "actions", "key actions":
			out.Actions = value
		case "conclusion":
			out.Conclusion = value
		case "follow-ups", "follow ups", "followups", "todos":
			out.FollowUps = value
		}
	}
	if strings.TrimSpace(out.Goal) == "" || strings.TrimSpace(out.Actions) == "" || strings.TrimSpace(out.Conclusion) == "" {
		return Elements{}, false
	}
	return out, true
}

// clipForRecap keeps the head and the tail of an over-long transcript: the head
// states what the session set out to do, the tail holds the conclusion.
func clipForRecap(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	head, tail := headPiece(text, max), tailPiece(text, max)
	return fmt.Sprintf("%s\n…[%d bytes omitted]…\n%s", head, len(text)-len(head)-len(tail), tail)
}

// headPiece is the head clipForRecap keeps, exposed on its own: a later read
// stores it, and the head of head+tail is the head of both.
func headPiece(text string, max int) string {
	if max <= 0 {
		return text
	}
	head := max * 3 / 5
	if head >= len(text) {
		return text
	}
	cut := text[:head]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return cut
}

// tailPiece is the tail clipForRecap keeps.
func tailPiece(text string, max int) string {
	if max <= 0 {
		return text
	}
	tail := max - max*3/5
	if tail >= len(text) {
		return text
	}
	cut := text[len(text)-tail:]
	if i := strings.IndexByte(cut, '\n'); i >= 0 {
		cut = cut[i+1:]
	}
	return cut
}
