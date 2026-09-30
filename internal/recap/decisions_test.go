package recap

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestHashEntryIsStableAndKindScoped(t *testing.T) {
	if HashEntry(KindFact, "the parser moved") != HashEntry(KindFact, " the parser moved ") {
		t.Fatal("surrounding space must not change a note's identity")
	}
	if HashEntry(KindFact, "same text") == HashEntry(KindRefuted, "same text") {
		t.Fatal("the same text under two kinds must not collide")
	}
}

func TestDecideAndClearRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Unix(1700000000, 0)

	if err := store.Decide(ctx, KindFact, "the parser moved", DecisionReject, now); err != nil {
		t.Fatalf("decide: %v", err)
	}
	decisions, err := store.Decisions(ctx)
	if err != nil {
		t.Fatalf("decisions: %v", err)
	}
	decision, ok := decisions[HashEntry(KindFact, "the parser moved")]
	if !ok || decision.Choice != DecisionReject || !decision.DecidedAt.Equal(now) {
		t.Fatalf("decision = %+v ok=%v", decision, ok)
	}
	rejected, err := store.Rejected(ctx)
	if err != nil {
		t.Fatalf("rejected: %v", err)
	}
	if !rejected[HashEntry(KindFact, "the parser moved")] {
		t.Fatalf("a rejected note must be reported as rejected: %+v", rejected)
	}

	// A later choice replaces the earlier one rather than adding a second row.
	if err := store.Decide(ctx, KindFact, "the parser moved", DecisionAccept, now.Add(time.Minute)); err != nil {
		t.Fatalf("re-decide: %v", err)
	}
	if got, err := store.DecisionFor(ctx, KindFact, "the parser moved"); err != nil || got.Choice != DecisionAccept {
		t.Fatalf("decision = %+v err=%v, want accept", got, err)
	}
	if rejected, _ := store.Rejected(ctx); rejected[HashEntry(KindFact, "the parser moved")] {
		t.Fatal("an accepted note must not stay rejected")
	}

	if err := store.ClearDecision(ctx, KindFact, "the parser moved"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := store.DecisionFor(ctx, KindFact, "the parser moved"); err == nil {
		t.Fatal("a cleared note must carry no decision")
	}
}

func TestGenerateDropsRejectedNotes(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	path := h.session(t, "20260101-000000.000000000-fake.jsonl", "one\n")

	first, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(first.Record.Entries) != 2 {
		t.Fatalf("want two notes to start from: %+v", first.Record.Entries)
	}
	dropped := first.Record.Entries[0]
	if err := h.store.Decide(ctx, dropped.Kind, dropped.Body, DecisionReject, time.Now()); err != nil {
		t.Fatalf("decide: %v", err)
	}

	// A rewritten session recaps again, and the rejected note must stay gone.
	if err := os.WriteFile(path, []byte("one\ntwo\nthree much longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := h.generator.Generate(ctx, path)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if !again.Stored || len(again.Record.Entries) != 1 {
		t.Fatalf("a rejected note must not resurface: %+v", again.Record.Entries)
	}
	if again.Record.Entries[0].Body == dropped.Body {
		t.Fatalf("the rejected note came back: %+v", again.Record.Entries[0])
	}
}
