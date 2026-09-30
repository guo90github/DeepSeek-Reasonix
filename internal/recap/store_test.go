package recap

import (
	"context"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), Options{InMemory: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	rec := Record{
		Path:        "/sessions/a.jsonl",
		Fingerprint: "abc123",
		Entries: []Entry{
			{Kind: KindFact, Body: "the parser moved", Evidence: "internal/parser.go"},
			{Kind: KindHandoff, Body: "the docs still describe the old syntax"},
		},
		Model:         "deepseek/deepseek-v4",
		PromptVersion: PromptVersion,
		GeneratedAt:   time.Unix(1700000000, 0),
	}
	if err := store.Put(ctx, rec); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok, err := store.Get(ctx, rec.Path)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Kind != KindFact || got.Entries[0].Body != "the parser moved" ||
		got.Entries[0].Evidence != "internal/parser.go" || got.Entries[1].Kind != KindHandoff ||
		got.Fingerprint != rec.Fingerprint || !got.GeneratedAt.Equal(rec.GeneratedAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: len=%d err=%v", len(list), err)
	}
	if err := store.Delete(ctx, rec.Path); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := store.Get(ctx, rec.Path); ok {
		t.Fatal("record survived deletion")
	}
}

func TestStorePendingClearedByPut(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	if err := store.MarkPending(ctx, "/sessions/a.jsonl", "boom", time.Now()); err != nil {
		t.Fatalf("mark pending: %v", err)
	}
	if err := store.MarkPending(ctx, "/sessions/a.jsonl", "boom again", time.Now()); err != nil {
		t.Fatalf("mark pending twice: %v", err)
	}
	records, pending, err := store.Counts(ctx)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if records != 0 || pending != 1 {
		t.Fatalf("counts = (%d,%d), want (0,1)", records, pending)
	}
	if err := store.Put(ctx, Record{Path: "/sessions/a.jsonl", Fingerprint: "f", PromptVersion: PromptVersion}); err != nil {
		t.Fatalf("put: %v", err)
	}
	records, pending, err = store.Counts(ctx)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if records != 1 || pending != 0 {
		t.Fatalf("counts after put = (%d,%d), want (1,0)", records, pending)
	}
}
