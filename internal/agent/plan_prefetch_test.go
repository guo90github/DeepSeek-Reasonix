package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// prefetchAgent builds the smallest agent the prefetch path needs: a registry
// with a read-only reader, the tier's tool armed for prefetch, and a session
// whose last assistant message names the given prose.
func prefetchAgent(t *testing.T, root, prose string, prefetchOn bool) *Agent {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(stubReadTool{name: "read_file", output: "contents of the file", readOnly: true})
	if prefetchOn {
		hostCalls := NewParallelCallsTool(reg)
		hostCalls.SetPrefetch(true)
		reg.Add(hostCalls)
	}
	a := &Agent{agentConfig: agentConfig{writeWorkspaceRoot: root}, svc: agentServices{tools: reg}}
	a.sess.conversation = &Session{Messages: []provider.Message{{Role: provider.RoleAssistant, Content: prose}}}
	return a
}

func readOutcome() []toolOutcome {
	return []toolOutcome{{output: "note", executed: true}}
}

// TestPlanExecuteTierParsingCoversPrefetch pins the three values: prefetch
// implies the surface, on does not imply the prefetch, and a typo stays off.
func TestPlanExecuteTierParsingCoversPrefetch(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		enabled, prefetch bool
	}{
		{raw: "", enabled: false, prefetch: false},
		{raw: "off", enabled: false, prefetch: false},
		{raw: "on", enabled: true, prefetch: false},
		{raw: "PREFETCH", enabled: true, prefetch: true},
		{raw: " prefetch ", enabled: true, prefetch: true},
		{raw: "yes", enabled: false, prefetch: false},
	} {
		tier := ParsePlanExecuteTier(tc.raw)
		if tier.Enabled() != tc.enabled || tier.Prefetch() != tc.prefetch {
			t.Fatalf("ParsePlanExecuteTier(%q) = %q (enabled=%v prefetch=%v), want enabled=%v prefetch=%v",
				tc.raw, tier, tier.Enabled(), tier.Prefetch(), tc.enabled, tc.prefetch)
		}
	}
}

// TestPrefetchReadsOnlyPathsTheModelNamed: a named, existing file that this
// round is not reading is prefetched; a missing one and one already being read
// are left alone, and the cap holds.
func TestPrefetchReadsOnlyPathsTheModelNamed(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha.go", "beta.go", "gamma.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prose := "I will look at alpha.go and beta.go, then gamma.go and nope.go."
	a := prefetchAgent(t, dir, prose, true)

	busy := []provider.ToolCall{{Name: "read_file", Arguments: `{"path":"beta.go"}`}}
	got := a.prefetchCandidates(busy)
	if len(got) != planPrefetchMaxPaths {
		t.Fatalf("candidates = %v, want the cap of %d", got, planPrefetchMaxPaths)
	}
	for _, want := range []string{"alpha.go", "gamma.go"} {
		if !containsString(got, want) {
			t.Fatalf("candidates = %v, want %q (beta.go is busy, nope.go missing)", got, want)
		}
	}
}

// TestPrefetchStaysQuietWithoutTheTier: the same named files produce nothing
// when the tier never armed the prefetch.
func TestPrefetchStaysQuietWithoutTheTier(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := prefetchAgent(t, dir, "next I read alpha.go", false)
	if iv := a.applyPlanPrefetch(context.Background(), nil, readOutcome(), 0); iv.fired() {
		t.Fatalf("a tier-less agent prefetched: %q", iv.guidance)
	}
}

// TestPrefetchMarksItselfAsNoReadReceipt: the tail says what the content is not,
// carries the file, and fires at most once per turn.
func TestPrefetchMarksItselfAsNoReadReceipt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := prefetchAgent(t, dir, "next I read alpha.go", true)

	iv := a.applyPlanPrefetch(context.Background(), nil, readOutcome(), 0)
	if !iv.fired() {
		t.Fatal("the prefetch never fired")
	}
	if !strings.Contains(iv.guidance, planPrefetchMarker) || !strings.Contains(iv.guidance, "alpha.go") {
		t.Fatalf("guidance = %q, want the marker and the file", iv.guidance)
	}
	if !strings.Contains(iv.guidance, "contents of the file") {
		t.Fatalf("guidance = %q, want the reader's output", iv.guidance)
	}
	if again := a.applyPlanPrefetch(context.Background(), nil, readOutcome(), 0); again.fired() {
		t.Fatalf("the prefetch fired twice in one turn: %q", again.guidance)
	}
}

// TestPrefetchNodesGoThroughTheReadOnlyCheck: the reader is resolved through the
// registry and refused when it can touch state, as parallel_calls does.
func TestPrefetchNodesGoThroughTheReadOnlyCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	reg.Add(stubReadTool{name: "read_file", output: "should not run", readOnly: false})
	hostCalls := NewParallelCallsTool(reg)
	hostCalls.SetPrefetch(true)
	reg.Add(hostCalls)
	a := &Agent{agentConfig: agentConfig{writeWorkspaceRoot: dir}, svc: agentServices{tools: reg}}
	a.sess.conversation = &Session{Messages: []provider.Message{{Role: provider.RoleAssistant, Content: "next I read alpha.go"}}}

	if _, err := a.readForPrefetch(context.Background(), "alpha.go"); err == nil {
		t.Fatal("a stateful reader was used for prefetch")
	}
}
