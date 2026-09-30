package recap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	failures, err := store.PendingMap(ctx)
	if err != nil {
		t.Fatalf("pending map: %v", err)
	}
	if got := failures["/sessions/a.jsonl"]; got.Attempts != 2 || got.Reason != "boom again" {
		t.Fatalf("pending = %+v, want 2 attempts and the latest reason", got)
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
	if failures, _ = store.PendingMap(ctx); len(failures) != 0 {
		t.Fatalf("a stored recap must clear the failure: %+v", failures)
	}
}

// The projection is a cache and must not grow without end — but only what it can
// rebuild is expendable. Decisions and unfinished items are the person's own
// output, and dropping either would undo work nobody asked to redo.
func TestPruneKeepsWhatThePersonProduced(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Unix(1800000000, 0)
	live := filepath.Join(t.TempDir(), "here.jsonl")
	missing := filepath.Join(t.TempDir(), "gone.jsonl")
	if err := os.WriteFile(live, []byte("x"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	stale := Record{Path: "/sessions/old.jsonl", Fingerprint: "f", PromptVersion: PromptVersion,
		GeneratedAt: now.Add(-recordRetention - time.Hour)}
	fresh := Record{Path: "/sessions/new.jsonl", Fingerprint: "f", PromptVersion: PromptVersion,
		GeneratedAt: now.Add(-time.Hour)}
	for _, rec := range []Record{stale, fresh} {
		if err := store.Put(ctx, rec); err != nil {
			t.Fatalf("put %s: %v", rec.Path, err)
		}
	}
	for _, path := range []string{live, missing} {
		if err := store.PutResume(ctx, path, Resume{Offset: 1, Head: "head", UserTurns: 1}); err != nil {
			t.Fatalf("put resume %s: %v", path, err)
		}
	}
	for i := 0; i < maxActivityRows+20; i++ {
		if err := store.Trace(ctx, "submit", "/sessions/a.jsonl", fmt.Sprintf("row %d", i), now); err != nil {
			t.Fatalf("trace: %v", err)
		}
	}
	if err := store.KeepOpen(ctx, OpenItem{Project: "/p", Body: "keep me"}, now); err != nil {
		t.Fatalf("keep open: %v", err)
	}
	if err := store.Decide(ctx, KindRefuted, "no, this was ruled out", DecisionReject, now); err != nil {
		t.Fatalf("decide: %v", err)
	}

	report, err := store.Prune(ctx, now)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if report.Records != 1 || report.Resumes != 1 || report.Activity != 20 {
		t.Fatalf("prune report = %+v, want one record, one resume marker, 20 log rows", report)
	}
	if _, ok, _ := store.Get(ctx, fresh.Path); !ok {
		t.Fatal("a record inside the window must survive")
	}
	if _, ok, _ := store.Get(ctx, stale.Path); ok {
		t.Fatal("a record past the window must go")
	}
	if _, ok, _ := store.Resume(ctx, live); !ok {
		t.Fatal("a resume marker whose session still exists must survive")
	}
	if _, ok, _ := store.Resume(ctx, missing); ok {
		t.Fatal("a resume marker whose session file is gone must go")
	}
	if items, _ := store.OpenItemsFor(ctx, "/p"); len(items) != 1 {
		t.Fatal("unfinished items are not the cache's to drop")
	}
	if rejected, _ := store.Rejected(ctx); len(rejected) != 1 {
		t.Fatal("decisions are not the cache's to drop")
	}
	if tail, err := store.Activity(ctx, maxActivityRows+50); err != nil || len(tail) != maxActivityRows {
		t.Fatalf("activity tail = %d (err=%v), want %d", len(tail), err, maxActivityRows)
	}
}
