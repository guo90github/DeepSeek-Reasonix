package memory

import (
	"fmt"
	"strings"
	"testing"
)

// The provider index is a first layer: one bounded glance per fact, plus a
// stable-id handle that fetches the body on demand (docs/50 §十). The glance
// cost must not grow with how long a fact's title or description is.
func TestProviderIndexFirstLayerStaysWithinItsBudget(t *testing.T) {
	store := recallTestStore(t)
	if _, err := store.Save(Memory{
		Name:        "bounded",
		Title:       strings.Repeat("很长的自动标题", 12),
		Description: strings.Repeat("description ", 20),
		Type:        TypeProject,
		Scope:       FactScopeProject,
		Body:        "the full body",
	}); err != nil {
		t.Fatal(err)
	}

	entry := strings.TrimSpace(store.Index())
	label := entry[strings.Index(entry, "[")+1 : strings.Index(entry, "]")]
	if runes := []rune(label); len(runes) > maxFirstLayerRunes {
		t.Fatalf("label exceeds the first-layer budget (%d runes): %q", len(runes), label)
	}
	if !strings.HasSuffix(label, "…") {
		t.Fatalf("a cut label must mark the cut: %q", label)
	}

	handle := entry[strings.Index(entry, "](")+2 : strings.Index(entry, ")")]
	m, ok := store.Read(handle)
	if !ok || m.Body != "the full body" {
		t.Fatalf("index handle %q did not resolve to the fact (ok=%v): %+v", handle, ok, m)
	}
}

func TestProviderIndexSummarySharesTheFirstLayerBudget(t *testing.T) {
	store := recallTestStore(t)
	if _, err := store.Save(Memory{
		Name:        "note",
		Description: strings.Repeat("summary ", 40),
		Type:        TypeProject,
		Scope:       FactScopeProject,
		Body:        "body",
	}); err != nil {
		t.Fatal(err)
	}

	entry := strings.TrimSpace(store.Index())
	label := entry[strings.Index(entry, "[")+1 : strings.Index(entry, "]")]
	tail := entry[strings.Index(entry, ") — [")+5:]
	summary := strings.TrimSpace(tail[strings.Index(tail, "]")+1:])
	if got := len([]rune(label)) + len([]rune(summary)); got > maxFirstLayerRunes {
		t.Fatalf("first layer spent %d runes, budget %d: %q", got, maxFirstLayerRunes, entry)
	}
	if !strings.HasSuffix(summary, "…") {
		t.Fatalf("a cut summary must mark the cut: %q", entry)
	}
}

func TestProviderIndexResidentCostPerFactIsBounded(t *testing.T) {
	store := recallTestStore(t)
	const facts = 20
	for i := range facts {
		if _, err := store.Save(Memory{
			Name:        fmt.Sprintf("fact-%02d", i),
			Description: strings.Repeat("a long second-layer description ", 12),
			Body:        "body",
		}); err != nil {
			t.Fatal(err)
		}
	}
	index := store.Index()
	if perFact := len(index) / facts; perFact > 200 {
		t.Fatalf("per-fact resident cost %d bytes exceeds the first-layer budget:\n%s", perFact, index)
	}
}
