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

const recapSystemPrompt = `You distill one finished coding session into reusable notes for the next session.
Answer with a JSON array and nothing else, like this:
[{"kind":"fact","body":"one or two sentences","evidence":"where it came from","refs":[{"kind":"path","value":"desktop/x.go","detail":"L40"},{"kind":"command","value":"go test ./internal/recap/"}],"scope":{"level":"project","reason":"only true in this repository"}}]

Kinds, and what earns a note:
- fact: something durable about the project as it now stands — where a thing
  lives, a path that matters later, a protocol, a schema, a field's meaning.
- root-cause: a defect that was actually diagnosed — the symptom, the root cause,
  and the fix, one clause each. No verification log, no command output.
- refuted: something the session tried, assumed, or reached for and then dropped
  — an approach, a tool, a version, a hypothesis — with what ruled it out. Most
  sessions drop something without arguing about it, and that silence is not a
  reason to skip the note. An option still under discussion is not refuted, and
  an approach the session simply used is not refuted either.
- handoff: work left unfinished and what the next session must do about it —
  including anything the session deferred with "later", "next time", or a TODO.

How to write one body:
- Prefer what only this session knows. The repo, its docs and its history answer
  their own questions on demand, so a note that restates a file, a commit or a
  document spends the next session's attention and returns nothing. What earns a
  note is what nothing else records: what the person decided or ruled out, a dead
  end already walked, the cause of something that went wrong, a constraint nobody
  wrote down. A fact the code already states plainly is not worth a note.
- Never state a term you cannot point at with "refs". If the session does not
  contain the wording verbatim, write the body so the uncertainty shows instead
  of asserting it: a note that is confidently a little wrong costs more than no
  note, because the next session quotes it as fact.
- Machines re-read their own state: a version, a path, a timestamp or a process
  count is worth a note only when the next session would otherwise re-derive it
  wrongly. Never dress such a thing as a root-cause.
- At most two sentences, and at most 120 characters. The next session reads this
  as a list, not as a report: a note that needs more room is two notes.
- At most two identifiers per note (a path, a command, an id). The rest belongs
  in "evidence" — a file, a command, a turn, or the user's own words. Never
  "turn N's step evidence" or any other reference to this session's bookkeeping.
- A note must stand without this session: no "as decided above", no "the earlier
  fix", no recounting of what was run, committed, or checked — unless the next
  session would break something without knowing it.
- Write each body in the session's own language.
- Copy names, identifiers, file paths, decision numbers, version strings and
  field values verbatim. Never paraphrase, translate, shorten or round them
  (a range stays a range: "O1-O9" is not "O1-O6").
- Report what the session settled on, never what it merely discussed.
- The transcript may be trimmed: never invent detail to fill a gap.
- No greetings, no restating the request, no narrating the conversation.
- Before answering, scan the session for three things: a defect that was
  diagnosed, anything it dropped along the way, and anything it left unfinished.
  Each hit is a note; never drop one because the list is getting long.
- Aim for three to six notes, never more than 8. A session that produced nothing
  reusable answers [].
- Never include secrets, credentials, hostnames, or internal addresses.`

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
		opts.MaxTokens = 3000
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
	case result.Stored && len(result.Record.Entries) == 0:
		return "stored: no reusable entries"
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
	raw, err := g.call(ctx, prov, ref, text)
	if err != nil {
		_ = store.MarkPending(ctx, path, err.Error(), g.opts.Now())
		return Result{Skipped: true, Reason: "call failed"}, nil
	}
	if strings.TrimSpace(raw) == "" {
		// A reasoning model can spend the whole completion budget thinking and
		// return nothing at all; one immediate retry, with room to finish, beats
		// leaving the session pending for a later sweep.
		raw, err = g.callWith(ctx, prov, ref, text, 2*g.opts.MaxTokens)
		if err != nil {
			_ = store.MarkPending(ctx, path, err.Error(), g.opts.Now())
			return Result{Skipped: true, Reason: "call failed"}, nil
		}
		if strings.TrimSpace(raw) == "" {
			// Name it for what it is: a model that answered nothing is not a parse
			// failure, and the two want different things from whoever reads the page.
			_ = store.MarkPending(ctx, path, "empty answer twice: the model returned nothing", g.opts.Now())
			return Result{Skipped: true, Reason: "empty answer"}, nil
		}
	}
	entries, ok := parseEntries(raw)
	if !ok {
		_ = store.MarkPending(ctx, path, "unparseable answer: "+answerExcerpt(raw), g.opts.Now())
		return Result{Skipped: true, Reason: "unparseable answer"}, nil
	}
	// Redaction runs first: a decision is keyed by the text a person actually
	// saw, which is the redacted body.
	entries = redactEntries(entries)
	if rejected, err := store.Rejected(ctx); err == nil {
		entries = dropRejected(entries, rejected)
	}
	rec := Record{
		Path:          path,
		Fingerprint:   fingerprint,
		Entries:       entries,
		Model:         ref,
		PromptVersion: PromptVersion,
		GeneratedAt:   g.opts.Now(),
	}
	if err := store.Put(ctx, rec); err != nil {
		return Result{}, err
	}
	return Result{Record: rec, Stored: true}, nil
}

// dropRejected removes the notes someone already ruled out, so recapping the
// same session does not resurface them.
func dropRejected(entries []Entry, rejected map[string]bool) []Entry {
	if len(rejected) == 0 {
		return entries
	}
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if rejected[HashEntry(entry.Kind, entry.Body)] {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// answerExcerpt keeps a short, redacted head of a rejected answer: without it a
// session that never recaps reports only "unparseable", and nothing says whether
// the model answered in prose or in a shape nothing here recognizes.
func answerExcerpt(raw string) string {
	text := strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
	if runes := []rune(text); len(runes) > 120 {
		text = string(runes[:120]) + "…"
	}
	return secrets.Redact(text)
}

// call runs the lane's one bounded request: no tools, its own usage source, and
// a completion budget the model's reasoning also has to fit inside.
func (g *Generator) call(ctx context.Context, prov provider.Provider, ref, text string) (string, error) {
	return g.callWith(ctx, prov, ref, text, g.opts.MaxTokens)
}

func (g *Generator) callWith(ctx context.Context, prov provider.Provider, ref, text string, maxTokens int) (string, error) {
	return boundedllm.Call(ctx, boundedllm.Config{
		Provider:    prov,
		ModelRef:    ref,
		Sink:        g.opts.Sink,
		UsageSource: event.UsageSourceSessionRecap,
		Timeout:     g.opts.Timeout,
		MaxTokens:   maxTokens,
		// Notes carry pointers and a tier now, so the answer is fatter than it was
		// when 8 KiB was enough; the salvage path only helps up to the first cut.
		MaxOutputBytes: 16 * 1024,
		MaxSystemBytes: 6 * 1024,
		MaxTotalBytes:  g.opts.MaxInputBytes + 4*1024,
		EffortOverride: provider.PreferredReasoning(prov, "low"),
	}, recapSystemPrompt, text)
}

// redactEntries scrubs a whole note: the body, the provenance it quotes, and the
// pointers it offers can each carry an address or a credential. Refs and Scope
// ride along — a note whose pointers are dropped on the way in cannot be checked
// by the next session, and v8 asks for those pointers by name.
func redactEntries(entries []Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, Entry{
			Kind:     entry.Kind,
			Body:     secrets.Redact(entry.Body),
			Evidence: secrets.Redact(entry.Evidence),
			Refs:     redactRefs(entry.Refs),
			Scope:    redactScope(entry.Scope),
		})
	}
	return out
}

func redactRefs(refs []Ref) []Ref {
	if len(refs) == 0 {
		return nil
	}
	out := make([]Ref, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Ref{
			Kind:   ref.Kind,
			Value:  secrets.Redact(ref.Value),
			Detail: secrets.Redact(ref.Detail),
		})
	}
	return out
}

func redactScope(scope Scope) Scope {
	scope.Reason = secrets.Redact(scope.Reason)
	return scope
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

// clipForRecap keeps the head and the tail of an over-long transcript — the head
// states what the session set out to do, the tail holds the conclusion — and
// turns the dropped middle into one line per turn. Losing the middle outright
// loses exactly the conclusions that were drawn in passing, which is what a
// person notices as "the recap missed the point".
func clipForRecap(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	head, tail := headPiece(text, max), tailPiece(text, max)
	middle := text[len(head) : len(text)-len(tail)]
	note := fmt.Sprintf("…[%d bytes omitted]…", len(middle))
	if digest := digestTurns(middle, omittedBudget(max)); digest != "" {
		note = fmt.Sprintf("…[%d bytes omitted; those turns came down to:]\n%s\n…[end of omitted middle]…",
			len(middle), digest)
	}
	return head + "\n" + note + "\n" + tail
}

// omittedBudget caps the digest well below the transcript budget: it is a map of
// what was dropped, not a second copy of it.
func omittedBudget(max int) int { return max / 5 }

// digestTurns renders one line per turn: the turn's own marker plus the first
// non-blank line of it. Markers come from the transcript renderer, so a text
// without them (a raw fragment) falls back to saying how much was dropped.
func digestTurns(middle string, budget int) string {
	lines := strings.Split(middle, "\n")
	turns := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			turns++
		}
	}
	var out []string
	used, kept := 0, 0
	marker := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			marker = trimmed
			continue
		}
		if trimmed == "" || marker == "" {
			continue
		}
		entry := marker + " " + firstClause(trimmed)
		if used+len(entry) > budget {
			break
		}
		out = append(out, entry)
		used += len(entry) + 1
		kept++
		marker = ""
	}
	if len(out) == 0 {
		return ""
	}
	if dropped := turns - kept; dropped > 0 {
		out = append(out, fmt.Sprintf("(+%d more turns)", dropped))
	}
	return strings.Join(out, "\n")
}

// firstClause is the readable start of one turn's first line, in runes so a
// multi-byte rune is never cut in half.
func firstClause(line string) string {
	const cap = 80
	runes := []rune(line)
	if len(runes) <= cap {
		return line
	}
	return string(runes[:cap]) + "…"
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
