package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/recap"
)

// Purging is permanent: the recap describes a session nobody can open again, and
// the age-based prune would keep that row for months. This pins the drop itself,
// on the same projection the app reads — the purge path calls it right after the
// session file is gone.
func TestDeletingAPurgedSessionsRecapDropsTheRow(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	ctx := context.Background()
	path := `C:\state\projects\alpha\sessions\20260101-000000.000000000-fake.jsonl`

	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		t.Fatalf("open the projection: %v", err)
	}
	if err := store.Put(ctx, recap.Record{Path: path, Fingerprint: "f", PromptVersion: recap.PromptVersion,
		GeneratedAt: time.Now(), Entries: []recap.Entry{{Kind: recap.KindFact, Body: "面板恒显示 0 个文件"}}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = store.Close()

	a := &App{}
	a.deleteSessionRecap(path)

	reopened, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	records, err := reopened.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("a purged session must leave no recap behind: %+v", records)
	}
	// An empty path is what a caller with no session passes; it must be a no-op
	// rather than deleting something else.
	a.deleteSessionRecap("")
}
