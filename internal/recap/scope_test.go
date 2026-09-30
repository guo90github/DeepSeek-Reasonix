package recap

import (
	"context"
	"testing"
	"time"
)

// A tier the model proposes is a guess; a conclusion two projects reached on their
// own is an observation. This is what tells them apart.
func TestRecurrencesFindAConclusionTwoBucketsReached(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Unix(1800000000, 0)

	shared := Entry{Kind: KindRefuted, Body: "只取分支统计未提交数会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go"}
	reached := func(bucket, path string, entries ...Entry) {
		if err := store.Put(ctx, Record{Path: path, Fingerprint: "f", PromptVersion: PromptVersion,
			GeneratedAt: now, Entries: entries}); err != nil {
			t.Fatalf("put %s: %v", path, err)
		}
	}
	reached(`C:\state\projects\alpha`, `C:\state\projects\alpha\sessions\a.jsonl`, shared)
	reached(`C:\state\projects\beta`, `C:\state\projects\beta\sessions\b.jsonl`,
		Entry{Kind: KindRefuted, Body: "统计未提交数只取分支会漏掉 CJK 路径",
			Evidence: "desktop/gitstats.go desktop/gitstats_test.go"})
	reached(`C:\state\projects\gamma`, `C:\state\projects\gamma\sessions\c.jsonl`,
		Entry{Kind: KindFact, Body: "打包脚本在 scripts/desktop-build.sh"})

	found, err := store.Recurrences(ctx)
	if err != nil {
		t.Fatalf("recurrences: %v", err)
	}
	buckets, ok := found[HashEntry(shared.Kind, shared.Body)]
	if !ok || len(buckets) != 2 {
		t.Fatalf("the conclusion two projects reached = %v (ok=%v), want both buckets", buckets, ok)
	}
	if others := ObservationOf(buckets, `C:\state\projects\alpha`); len(others) != 1 || others[0] != "beta" {
		t.Fatalf("a person reads the other project, labelled: %v", others)
	}
	if _, ok := found[HashEntry(KindFact, "打包脚本在 scripts/desktop-build.sh")]; ok {
		t.Fatal("a conclusion only one project reached is not a recurrence")
	}
	// Wording alone is not a subject: two unrelated notes that happen to share a
	// phrase must not be read as the same conclusion.
	reached(`C:\state\projects\delta`, `C:\state\projects\delta\sessions\d.jsonl`,
		Entry{Kind: KindFact, Body: "只取分支统计未提交数会漏掉 CJK 路径"})
	found, err = store.Recurrences(ctx)
	if err != nil {
		t.Fatalf("recurrences: %v", err)
	}
	// It shares identifiers with the alpha note, so it does count — the same
	// evidence the offer rule uses, no second standard.
	if len(found[HashEntry(KindFact, "只取分支统计未提交数会漏掉 CJK 路径")]) != 3 {
		t.Fatalf("same evidence, same rule: %v", found)
	}
}

func TestBucketLabelNamesWhatAPersonReads(t *testing.T) {
	if got := BucketLabel(`C:\state\projects\c--guosj-ai-repo`); got != "c--guosj-ai-repo" {
		t.Fatalf("label = %q", got)
	}
	if got := BucketLabel(`C:\state\projects\alpha\`); got != "alpha" {
		t.Fatalf("a trailing separator must not become the label: %q", got)
	}
	if got := BucketLabel("   "); got != "" {
		t.Fatalf("an empty bucket has no label, got %q", got)
	}
}
