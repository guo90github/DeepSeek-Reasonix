package recap

import (
	"context"
	"testing"
	"time"
)

func TestProjectOfGroupsSessionsByProject(t *testing.T) {
	project := "C:\\state\\projects\\c--guosj-ai-repo"
	if got := ProjectOf(project + "\\sessions\\20260101-000000.000000000-m.jsonl"); got != project {
		t.Fatalf("ProjectOf = %q, want the project directory %q", got, project)
	}
	if got := ProjectOf("C:\\state\\sessions\\20260101-000000.000000000-m.jsonl"); got != "C:\\state" {
		t.Fatalf("a global session belongs to the state root, got %q", got)
	}
	if got := ProjectOf("  "); got != "" {
		t.Fatalf("an empty path names no project, got %q", got)
	}
}

func TestMatchOpenItemsReadsTheSubjectNotTheProse(t *testing.T) {
	items := []OpenItem{
		{ID: "a", Project: "/p", Body: "回归测试仍缺：给 Git 未提交面板补一个用例", Evidence: "desktop/workspace_git_scope_test.go"},
		{ID: "b", Project: "/p", Body: "文档还没更新"},
		{ID: "c", Project: "/p", Body: "已处理的那条", ClosedAt: time.Now()},
	}
	// Same subject, said a little differently: the shared pieces carry it.
	if got := MatchOpenItems(items, "我们接着把 Git 未提交面板的用例补上吧"); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("a subject match must be offered: %+v", got)
	}
	// One shared pair of characters is ordinary prose, not a subject.
	if got := MatchOpenItems(items, "测试环境又挂了，帮我看看日志"); len(got) != 0 {
		t.Fatalf("a single shared pair must not match: %+v", got)
	}
	// A different subject in the same project stays silent: handing this work to
	// an unrelated session is the failure this rule exists to prevent.
	if got := MatchOpenItems(items, "帮我看看主题颜色为什么偏暗"); len(got) != 0 {
		t.Fatalf("another subject must not match: %+v", got)
	}
	// An identifier in the turn is decisive on its own.
	if got := MatchOpenItems(items, "看下 desktop/workspace_git_scope_test.go"); len(got) != 1 {
		t.Fatalf("an identifier match must be offered: %+v", got)
	}
	// A closed item is never offered again.
	if got := MatchOpenItems(items, "已处理的那条还是有问题，要继续"); len(got) != 0 {
		t.Fatalf("a closed item must not be offered: %+v", got)
	}
	if got := MatchOpenItems(items, "   "); len(got) != 0 {
		t.Fatalf("an empty turn matches nothing: %+v", got)
	}
}

func TestMatchOpenItemsIsBounded(t *testing.T) {
	items := make([]OpenItem, 0, 8)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		items = append(items, OpenItem{ID: id, Body: "未完成项 " + id})
	}
	if got := MatchOpenItems(items, "未完成项 a b c d e"); len(got) != maxMatchedItems {
		t.Fatalf("an offer is capped at %d items, got %d", maxMatchedItems, len(got))
	}
}

func TestKeepAndCloseOpenItems(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	project := "/state/projects/alpha"
	item := OpenItem{Project: project, Body: "给 X 模块补回归测试", Evidence: "desktop/x.go", From: project + "/sessions/a.jsonl"}

	if err := store.KeepOpen(ctx, item, time.Unix(1700000000, 0)); err != nil {
		t.Fatalf("keep open: %v", err)
	}
	// Keeping the same note twice must not duplicate it.
	if err := store.KeepOpen(ctx, item, time.Unix(1700000100, 0)); err != nil {
		t.Fatalf("keep open again: %v", err)
	}
	open, err := store.OpenItemsFor(ctx, project)
	if err != nil || len(open) != 1 {
		t.Fatalf("open items = %+v err=%v, want exactly one", open, err)
	}
	if open[0].ID == "" || open[0].Project != project || !open[0].Open() {
		t.Fatalf("stored item lost its identity: %+v", open[0])
	}
	if other, _ := store.OpenItemsFor(ctx, "/state/projects/beta"); len(other) != 0 {
		t.Fatalf("another project must not see this item: %+v", other)
	}

	if err := store.CloseOpen(ctx, open[0].ID, time.Unix(1700000200, 0)); err != nil {
		t.Fatalf("close: %v", err)
	}
	if still, _ := store.OpenItemsFor(ctx, project); len(still) != 0 {
		t.Fatalf("a closed item must leave the open list: %+v", still)
	}
	all, err := store.OpenItemsForProject(ctx, project)
	if err != nil || len(all) != 1 || all[0].Open() {
		t.Fatalf("a closed item is kept for the record: %+v err=%v", all, err)
	}

	if err := store.ReopenOpen(ctx, open[0].ID); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if again, _ := store.OpenItemsFor(ctx, project); len(again) != 1 {
		t.Fatalf("reopening restores the item: %+v", again)
	}
}
