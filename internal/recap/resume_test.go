package recap

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// coveredTranscript counts which read the lane actually took: the replay through
// Read, or a covered read attempt. covered counts attempts, including one that
// declines, so a test can tell an attempt from a replay.
type coveredTranscript struct {
	mu      sync.Mutex
	full    int
	covered int
	text    string
	next    Resume
	refuse  bool
}

func (r *coveredTranscript) Read(context.Context, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.full++
	return "replayed: " + r.text, nil
}

func (r *coveredTranscript) ReadCovered(context.Context, string, Resume, int) (string, Resume, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.covered++
	if r.refuse {
		return "", Resume{}, false, nil
	}
	return r.text, r.next, true, nil
}

func (r *coveredTranscript) counts() (full, covered int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.full, r.covered
}

// The lane must prefer the covered read when a previous close recorded one: that
// is the whole point of the phase, so it is asserted where it takes effect.
func TestGenerateTakesTheCoveredReadAndRecordsWhereItReached(t *testing.T) {
	ctx := context.Background()
	transcript := &coveredTranscript{
		text: "covered text",
		next: Resume{Offset: 10, Hash: "hash", Digest: "digest", Head: "head", UserTurns: 1},
	}
	h := newHarness(t, func(o *GeneratorOptions) { o.Transcript = transcript })
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	if _, err := h.generator.Generate(ctx, path); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if full, covered := transcript.counts(); covered != 1 || full != 0 {
		t.Fatalf("first close took covered=%d full=%d, want the covered read once", covered, full)
	}
	recorded, ok, err := h.store.Resume(ctx, path)
	if err != nil || !ok {
		t.Fatalf("the read was not recorded: ok=%v err=%v", ok, err)
	}
	if recorded.Offset != 10 || recorded.Hash != "hash" || recorded.Head != "head" || recorded.UserTurns != 1 {
		t.Fatalf("recorded resume = %+v", recorded)
	}

	// An unchanged session is skipped, so make it dirty to force a second read.
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.generator.Generate(ctx, path); err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if full, covered := transcript.counts(); covered != 2 || full != 0 {
		t.Fatalf("second close took covered=%d full=%d, want the covered read again", covered, full)
	}
}

func TestGenerateFallsBackToTheReplayWhenNoReadIsProven(t *testing.T) {
	ctx := context.Background()
	transcript := &coveredTranscript{refuse: true}
	h := newHarness(t, func(o *GeneratorOptions) { o.Transcript = transcript })
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	if _, err := h.generator.Generate(ctx, path); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if full, covered := transcript.counts(); full != 1 || covered != 1 {
		t.Fatalf("took full=%d covered=%d, want one attempt each", full, covered)
	}
	if _, ok, _ := h.store.Resume(ctx, path); ok {
		t.Fatal("a refused covered read must not record a resume point")
	}
}

// A covered read is only allowed when the file still holds what the previous
// close rendered, so the two cases where it does not must keep the full read.
func TestCoveredReadMatchesTheFullReadAndRefusesUnprovenGrowth(t *testing.T) {
	ctx := context.Background()
	reader := FileTranscript{}
	const max = 1 << 20

	path := savedSession(t, t.TempDir())
	full, next, ok, err := reader.ReadCovered(ctx, path, Resume{}, max)
	if err != nil || !ok {
		t.Fatalf("full read: ok=%v err=%v", ok, err)
	}
	if next.Offset <= 0 || next.Hash == "" || next.Digest == "" || next.UserTurns != 1 {
		t.Fatalf("resume baseline = %+v", next)
	}

	// A real save appends a turn and advances the sidecar digest.
	ses, err := agent.LoadSession(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ses.Add(provider.Message{Role: provider.RoleUser, Content: "second question"})
	ses.Add(provider.Message{Role: provider.RoleAssistant, Content: "second answer"})
	if err := ses.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	grown, advanced, ok, err := reader.ReadCovered(ctx, path, next, max)
	if err != nil || !ok {
		t.Fatalf("covered read: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(grown, "second answer") {
		t.Fatalf("covered read lost the new turn: %q", grown)
	}
	after, _, ok, err := reader.ReadCovered(ctx, path, Resume{}, max)
	if err != nil || !ok {
		t.Fatalf("second full read: ok=%v err=%v", ok, err)
	}
	if grown != after {
		t.Fatalf("covered read disagrees with the full read:\ncovered: %q\nfull:    %q", grown, after)
	}
	if advanced.Offset <= next.Offset || advanced.UserTurns != 2 {
		t.Fatalf("resume did not advance: %+v", advanced)
	}
	if advanced.Head != headPiece(after, max) {
		t.Fatalf("kept head = %q, want %q", advanced.Head, headPiece(after, max))
	}
	if !strings.HasPrefix(grown, full) {
		t.Fatalf("covered read did not build on the earlier one:\ncovered: %q\nearlier: %q", grown, full)
	}

	// A row appended without a save leaves the digest stale: keep the full read.
	appended := savedSession(t, t.TempDir())
	if _, next, ok, _ := reader.ReadCovered(ctx, appended, Resume{}, max); !ok {
		t.Fatal("baseline for the appended case was not proven")
	} else {
		appendRow(t, appended, provider.Message{Role: provider.RoleUser, Content: "appended without saving"})
		if _, _, ok, _ := reader.ReadCovered(ctx, appended, next, max); ok {
			t.Fatal("growth without a recorded save must keep the full read")
		}
	}

	// A rewritten opening changes the prefix the previous read covered.
	rewritten := savedSession(t, t.TempDir())
	_, next, ok, _ = reader.ReadCovered(ctx, rewritten, Resume{}, max)
	if !ok {
		t.Fatal("baseline for the rewritten case was not proven")
	}
	other := agent.NewSession("system")
	other.Add(provider.Message{Role: provider.RoleUser, Content: "a completely different opening question"})
	other.Add(provider.Message{Role: provider.RoleAssistant, Content: "different answer"})
	if err := other.Save(rewritten); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if _, _, ok, _ := reader.ReadCovered(ctx, rewritten, next, max); ok {
		t.Fatal("a rewritten prefix must keep the full read")
	}
}

func TestStoreResumeRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	want := Resume{Offset: 4096, Hash: "abc", Digest: "def", Head: "head", UserTurns: 3}
	if err := store.PutResume(ctx, "/sessions/a.jsonl", want); err != nil {
		t.Fatalf("put resume: %v", err)
	}
	got, ok, err := store.Resume(ctx, "/sessions/a.jsonl")
	if err != nil || !ok || got != want {
		t.Fatalf("resume = %+v ok=%v err=%v", got, ok, err)
	}
	if err := store.Delete(ctx, "/sessions/a.jsonl"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := store.Resume(ctx, "/sessions/a.jsonl"); ok {
		t.Fatal("resume point survived deletion")
	}
}
