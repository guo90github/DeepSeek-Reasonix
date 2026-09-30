package recap

import (
	"context"
	"testing"
	"time"
)

func TestPriorNotesCarryOnlyUndecidedConclusionsOfTheBucket(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	bucket := `C:\state\projects\alpha`
	mine := bucket + `\sessions\a.jsonl`
	theirs := `C:\state\projects\beta\sessions\b.jsonl`
	now := time.Unix(1800000000, 0)

	if err := store.Put(ctx, Record{Path: mine, Fingerprint: "f", PromptVersion: PromptVersion,
		GeneratedAt: now, Entries: []Entry{
			{Kind: KindRefuted, Body: "只取分支统计未提交数会漏掉 CJK 路径", Evidence: "desktop/gitstats.go"},
			{Kind: KindRootCause, Body: "面板恒显示 0 个文件是只取分支造成的", Evidence: "internal/panel.go"},
			{Kind: KindFact, Body: "解析器搬到了 internal/parser.go"},
			{Kind: KindHandoff, Body: "文档还没更新"},
		}}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Put(ctx, Record{Path: theirs, Fingerprint: "f", PromptVersion: PromptVersion,
		GeneratedAt: now, Entries: []Entry{{Kind: KindRefuted, Body: "别的项目里否证了 B 方案"}}}); err != nil {
		t.Fatalf("put other project: %v", err)
	}
	// A decided note never comes back: an accepted one already lives in memory, and
	// a dropped one is a promise the drop button made.
	if err := store.Decide(ctx, KindRootCause, "面板恒显示 0 个文件是只取分支造成的", DecisionReject, now); err != nil {
		t.Fatalf("decide: %v", err)
	}

	notes, err := store.PriorNotes(ctx, bucket, 10)
	if err != nil {
		t.Fatalf("prior notes: %v", err)
	}
	if len(notes) != 1 || notes[0].Kind != KindRefuted {
		t.Fatalf("prior notes = %+v, want only the undecided refuted conclusion", notes)
	}
	if notes[0].From != mine || notes[0].Evidence != "desktop/gitstats.go" {
		t.Fatalf("a carried note must keep its origin: %+v", notes[0])
	}
	if capped, _ := store.PriorNotes(ctx, bucket, 1); len(capped) != 1 {
		t.Fatalf("the cap must hold, got %d", len(capped))
	}
	if leak, _ := store.PriorNotes(ctx, "/state/projects/gamma", 10); len(leak) != 0 {
		t.Fatalf("another project's conclusions must not leak: %+v", leak)
	}
}

func TestMatchNotesUsesTheSameEvidenceAsItems(t *testing.T) {
	notes := []Note{{
		Kind:     KindRefuted,
		Body:     "只取分支统计未提交数会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go desktop/gitstats_cache.go",
	}}
	subject := "desktop/gitstats.go、desktop/gitstats_test.go、desktop/gitstats_cache.go 都要改"

	if got := MatchNotes(notes, "desktop/gitstats.go 和 desktop/gitstats_test.go 都要改", 0); len(got) != 0 {
		t.Fatalf("two named files must not be enough, same bar as the items: %+v", got)
	}
	if got := MatchNotes(notes, subject, 0); len(got) != 1 || got[0].Kind != KindRefuted {
		t.Fatalf("a note on the subject must be carried: %+v", got)
	}
	if got := MatchNotes(notes, "帮我看看主题颜色为什么偏暗", 0); len(got) != 0 {
		t.Fatalf("another subject must stay silent: %+v", got)
	}
	// A short overlap of wording is ordinary, wherever it appears; a verbatim
	// repeat of the whole conclusion is the subject and is carried.
	if got := MatchNotes(notes, "只取分支的做法要改", 0); len(got) != 0 {
		t.Fatalf("two shared pieces must not carry a note: %+v", got)
	}
	if got := MatchNotes(notes, "只取分支统计未提交数会漏掉 CJK 路径", 0); len(got) != 1 {
		t.Fatalf("a verbatim repeat of the conclusion is the subject: %+v", got)
	}
	if got := StrongMatchNotes(notes, subject, 0); len(got) != 1 {
		t.Fatalf("named things must carry a note mid-conversation: %+v", got)
	}
	if got := StrongMatchNotes(notes, "只取分支统计未提交数会漏掉 CJK 路径", 0); len(got) != 0 {
		t.Fatalf("prose alone must not carry a note mid-conversation: %+v", got)
	}
	if got := MatchNotes(notes, subject, 0); len(got) != 1 {
		t.Fatal("a zero cap means no cap, not no match")
	}
}
