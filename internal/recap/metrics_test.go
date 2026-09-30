package recap

import (
	"context"
	"testing"
	"time"
)

// The numbers are the point: a channel that only ever says "it works" is a channel
// nobody can steer. This pins what is counted, so a later change that quietly
// stops counting something fails here instead of in a report.
func TestMetricsCountWhatTheChannelProduced(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Unix(1800000000, 0)
	alpha := `C:\state\projects\alpha`
	beta := `C:\state\projects\beta`

	shared := Entry{Kind: KindRefuted, Body: "只取分支统计未提交数会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go",
		Refs:     []Ref{{Kind: RefPath, Value: "desktop/gitstats.go"}}}
	alsoInBeta := Entry{Kind: KindRefuted, Body: "统计未提交数只取分支会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go"}
	onlyHere := Entry{Kind: KindFact, Body: "面板恒显示 0 个文件"}
	put := func(path string, entries ...Entry) {
		t.Helper()
		if err := store.Put(ctx, Record{Path: path, Fingerprint: "f", PromptVersion: PromptVersion,
			GeneratedAt: now, Entries: entries}); err != nil {
			t.Fatalf("put %s: %v", path, err)
		}
	}
	put(alpha+`\sessions\a.jsonl`, shared, onlyHere)
	put(beta+`\sessions\b.jsonl`, alsoInBeta)
	if err := store.Decide(ctx, KindRefuted, shared.Body, DecisionAccept, now); err != nil {
		t.Fatalf("decide accept: %v", err)
	}
	if err := store.Decide(ctx, KindFact, onlyHere.Body, DecisionReject, now); err != nil {
		t.Fatalf("decide reject: %v", err)
	}

	got, err := store.MetricsOf(ctx, now)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if got.Records != 2 || got.Notes != 3 {
		t.Fatalf("records=%d notes=%d, want 2 and 3", got.Records, got.Notes)
	}
	if got.Summary() != "fact=1 refuted=2" {
		t.Fatalf("kinds = %q", got.Summary())
	}
	if got.WithPointers != 1 {
		t.Fatalf("notes with pointers = %d, want the one that cited a place to check", got.WithPointers)
	}
	if got.Accepted != 1 || got.Rejected != 1 {
		t.Fatalf("decisions = %d accepted / %d rejected, want one of each", got.Accepted, got.Rejected)
	}
	if got.ObservedBeyond != 2 {
		t.Fatalf("observed beyond their own project = %d, want both refuted notes", got.ObservedBeyond)
	}
	if got.AvgBodyRunes == 0 || got.OldestDays != 0 {
		t.Fatalf("avg body = %d runes, oldest = %d days", got.AvgBodyRunes, got.OldestDays)
	}
	if got.Percent(got.WithPointers, got.Notes) != "33%" || got.Percent(1, 0) != "n/a" {
		t.Fatalf("percentages = %q / %q", got.Percent(got.WithPointers, got.Notes), got.Percent(1, 0))
	}
}
