package recap

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

// windowModels is a resolver that also knows the model's context window: the
// shape the lane's own WindowResolver seam exists for.
type windowModels struct {
	inner  ModelResolver
	window int
}

func (m windowModels) Resolve(ctx context.Context, sessionPath string) (provider.Provider, string, bool) {
	return m.inner.Resolve(ctx, sessionPath)
}

func (m windowModels) WindowTokens(string) int { return m.window }

// A long session must be measured against the model's window, not against a
// constant: that constant is what reduced every long session to a fixed slice.
func TestEvidenceBudgetFollowsTheModelWindow(t *testing.T) {
	floor := recapDefaultMaxInputBytes
	cases := []struct {
		name   string
		models ModelResolver
		want   func(int) bool
		why    string
	}{
		{
			name:   "no window known",
			models: fakeModels{ok: true},
			want:   func(got int) bool { return got == floor },
			why:    "an unknown window must leave the lane at the budget it always spent",
		},
		{
			name:   "tiny window",
			models: windowModels{inner: fakeModels{ok: true}, window: 2_000},
			want:   func(got int) bool { return got == floor },
			why:    "a window narrower than the floor must not shrink the lane below it",
		},
		{
			name:   "a million tokens",
			models: windowModels{inner: fakeModels{ok: true}, window: 1_000_000},
			want: func(got int) bool {
				return got > floor*4 && got <= recapHardMaxInputBytes
			},
			why: "the configured window must raise the budget well past the floor",
		},
		{
			name:   "an enormous window",
			models: windowModels{inner: fakeModels{ok: true}, window: 100_000_000},
			want:   func(got int) bool { return got == recapHardMaxInputBytes },
			why:    "the lane's appetite is bounded whatever the model claims",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(GeneratorOptions{
				Models:     tc.models,
				Store:      openTestStore(t),
				Transcript: fakeTranscript{text: "x"},
			})
			if got := g.evidenceBudget("fake/model"); !tc.want(got) {
				t.Fatalf("evidenceBudget = %d, want %s", got, tc.why)
			}
		})
	}
	// A resolver that cannot answer still leaves the floor standing.
	g := NewGenerator(GeneratorOptions{Models: fakeModels{ok: true}, Store: openTestStore(t),
		Transcript: fakeTranscript{text: "x"}})
	if got := g.evidenceBudget("fake/model"); got != floor {
		t.Fatalf("evidenceBudget without a window = %d, want %d", got, floor)
	}
}

func bigTranscript(target int) string {
	var b strings.Builder
	for i := 1; b.Len() < target; i++ {
		fmt.Fprintf(&b, "## User (turn %d)\n这一轮我在查打包脚本里的路径处理，顺便核对了 NSIS 与 portable 目录的约定。\n\n"+
			"## Assistant (turn %d)\n我先读了 scripts/desktop-build.sh，然后核对 build.json 的 commit 字段。\n\n", i, i)
	}
	return b.String()
}

// The point of deriving the budget: a session that fits the model's window is sent
// whole, middle included, instead of being clipped to the lane's old constant.
func TestGenerateSendsAWholeSessionThatFitsTheModelWindow(t *testing.T) {
	const window = 1_000_000
	text := bigTranscript(1200 * 1024)
	if len(text) < 1024*1024 {
		t.Fatalf("fixture too small to prove anything: %d bytes", len(text))
	}
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = windowModels{inner: o.Models, window: window}
		o.Transcript = fakeTranscript{text: text}
	})
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", text)

	res, err := h.generator.Generate(context.Background(), path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !res.Stored {
		t.Fatalf("a session inside the window must still be recapped: %+v", res)
	}
	asked := h.provider.lastEvidence()
	if len(asked) != len(text) {
		t.Fatalf("evidence = %d bytes, want the whole %d-byte transcript (no clip at this window)",
			len(asked), len(text))
	}
}

// A provider that refuses an oversized request must not turn the bigger window
// into a new hard failure: the lane retries smaller, which is the one thing it can
// change about what it sends.
func TestGenerateRetriesSmallerWhenTheProviderRefusesTheRequest(t *testing.T) {
	const window = 1_000_000
	text := bigTranscript(3 * 1024 * 1024)
	h := newHarness(t, func(o *GeneratorOptions) {
		o.Models = windowModels{inner: o.Models, window: window}
		o.Transcript = fakeTranscript{text: text}
	})
	h.provider.failures = 1
	// One refusal, then an answer: the retry is what this test is about.
	h.provider.answer = goodAnswer
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", text)

	res, err := h.generator.Generate(context.Background(), path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !res.Stored {
		t.Fatalf("the smaller retry must still store a recap: %+v", res)
	}
	asked := len(h.provider.lastEvidence())
	first := h.generator.evidenceBudget("fake/model")
	if asked == 0 {
		t.Fatal("the retry sent no evidence at all")
	}
	if asked > first/2+recapGuardSlack {
		t.Fatalf("retry sent %d bytes, want at most half of the %d-byte budget", asked, first)
	}
	if asked <= recapDefaultMaxInputBytes {
		t.Fatalf("retry fell back to the floor (%d bytes) instead of half the window budget", asked)
	}
}
