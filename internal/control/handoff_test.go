package control

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

// handoffController builds a controller whose project has one unfinished item.
func handoffController(t *testing.T, path string, items []recap.OpenItem, withLoader bool, priorTurns int) *Controller {
	t.Helper()
	session := agent.NewSession("sys")
	for range priorTurns {
		session.Add(provider.Message{Role: provider.RoleUser, Content: "earlier turn"})
	}
	opts := Options{
		Executor:    agent.New(nil, nil, session, agent.Options{}, event.Discard),
		SessionDir:  filepath.Dir(path),
		SessionPath: path,
		Label:       "test",
	}
	if withLoader {
		opts.ProjectOffers = func(string) recap.Offers { return recap.Offers{Items: items} }
	}
	return New(opts)
}

func TestComposeOffersAnUnfinishedItemOnlyWhenTheTurnIsAboutIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions", "20260101-000000.000000000-m.jsonl")
	items := []recap.OpenItem{{ID: "a", Project: recap.ProjectOf(path), Body: "给 Git 未提交面板补一个用例", OpenedAt: time.Now()}}

	// The session's first turn, about the item: offered as a question.
	first := handoffController(t, path, items, true, 0).Compose("接着把 Git 未提交面板的用例补上")
	if !strings.Contains(first, "<open-items>") || !strings.Contains(first, "Git 未提交面板") {
		t.Fatalf("a matching first turn must be offered the item: %q", first)
	}
	if !strings.HasSuffix(strings.TrimSpace(first), "接着把 Git 未提交面板的用例补上") {
		t.Fatalf("the offer must ride the turn tail, not replace it: %q", first)
	}

	// The first turn is offered on its subject alone, with no continuation cue:
	// opening a session about unfinished work is the case this exists for.
	opening := handoffController(t, path, items, true, 0).Compose("Git 未提交面板的用例该怎么补")
	if !strings.Contains(opening, "<open-items>") {
		t.Fatalf("a first turn on the subject must be offered the item: %q", opening)
	}

	// A first turn about something else stays silent: an unrelated session must
	// never be handed this work.
	if other := handoffController(t, path, items, true, 0).Compose("帮我看看主题颜色为什么偏暗"); strings.Contains(other, "<open-items>") {
		t.Fatalf("an unrelated first turn must not be offered work: %q", other)
	}

	// A later turn that does not say it continues anything stays silent, even on
	// the same subject.
	if later := handoffController(t, path, items, true, 3).Compose("Git 未提交面板的用例"); strings.Contains(later, "<open-items>") {
		t.Fatalf("a later turn without a continuation cue must not be offered work: %q", later)
	}

	// A later turn that does say it continues something is offered it.
	continued := handoffController(t, path, items, true, 3).Compose("接着把 Git 未提交面板的用例补上")
	if !strings.Contains(continued, "<open-items>") {
		t.Fatalf("a continuation turn must be offered the item: %q", continued)
	}

	// An item nobody picked up for a month stops being offered on its own: that
	// is what keeps a growing list from turning into wrong handoffs. It stays on
	// the project's list, and an explicit rollback puts it back in play.
	stale := []recap.OpenItem{{
		ID: "a", Project: recap.ProjectOf(path), Body: "给 Git 未提交面板补一个用例",
		OpenedAt: time.Now().Add(-40 * 24 * time.Hour),
	}}
	if old := handoffController(t, path, stale, true, 0).Compose("接着把 Git 未提交面板的用例补上"); strings.Contains(old, "<open-items>") {
		t.Fatalf("an item past the window must not be offered: %q", old)
	}
	revived := []recap.OpenItem{{
		ID: "a", Project: recap.ProjectOf(path), Body: "给 Git 未提交面板补一个用例",
		OpenedAt: time.Now(),
	}}
	if again := handoffController(t, path, revived, true, 0).Compose("接着把 Git 未提交面板的用例补上"); !strings.Contains(again, "<open-items>") {
		t.Fatalf("re-keeping the item must offer it again: %q", again)
	}

	// With no items kept, or no loader at all, nothing is ever offered.
	if none := handoffController(t, path, nil, true, 0).Compose("接着把 Git 未提交面板的用例补上"); strings.Contains(none, "<open-items>") {
		t.Fatalf("a project with no unfinished items must stay quiet: %q", none)
	}
	if bare := handoffController(t, path, items, false, 0).Compose("接着把 Git 未提交面板的用例补上"); strings.Contains(bare, "<open-items>") {
		t.Fatalf("without a loader the offer must be inert: %q", bare)
	}
}

// Mid-conversation the offer runs on a named thing instead of the subject, so a
// session that got there some other way is still picked up — and a session that
// only shares wording is not.
func TestComposeOffersAMidConversationTurnThatNamesTheItem(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions", "20260101-000000.000000000-m.jsonl")
	items := []recap.OpenItem{{
		ID: "a", Project: recap.ProjectOf(path), Body: "给 Git 未提交面板补一个用例",
		Evidence: "desktop/workspace_git_scope_test.go", OpenedAt: time.Now(),
	}}

	if prose := handoffController(t, path, items, true, 3).Compose("把 Git 未提交面板的用例补上"); strings.Contains(prose, "<open-items>") {
		t.Fatalf("mid-conversation, the subject alone must stay quiet: %q", prose)
	}
	if bare := handoffController(t, path, items, true, 3).Compose("desktop/workspace_git_scope_test.go 这个还是红的"); strings.Contains(bare, "<open-items>") {
		t.Fatalf("mid-conversation, one named file alone must stay quiet: %q", bare)
	}
	named := handoffController(t, path, items, true, 3).Compose("desktop/workspace_git_scope_test.go 里那个未提交面板的用例还是红的")
	if !strings.Contains(named, "<open-items>") {
		t.Fatalf("mid-conversation, naming the item's file with prose agreement must be offered it: %q", named)
	}
	if first := handoffController(t, path, items, true, 0).Compose("把 Git 未提交面板的用例补上"); !strings.Contains(first, "<open-items>") {
		t.Fatalf("a first turn on the subject must still be offered the item: %q", first)
	}
}

func TestHasContinuationCueReadsThePhrasing(t *testing.T) {
	for _, text := range []string{"接着上次那条", "继续把未完成项做完", "上回那个 bug 还在吗"} {
		if !hasContinuationCue(text) {
			t.Fatalf("%q says it continues something", text)
		}
	}
	for _, text := range []string{"帮我看看主题颜色", "今天天气不错", ""} {
		if hasContinuationCue(text) {
			t.Fatalf("%q does not say it continues anything", text)
		}
	}
}
