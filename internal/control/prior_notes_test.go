package control

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/recap"
)

// Earlier conclusions ride the same gates as the unfinished items but are shaped
// as background: the person never asked for the line, so a wrong one has to be
// ignorable instead of being handed work.
func TestComposeCarriesEarlierConclusionsAsBackground(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions", "20260101-000000.000000000-m.jsonl")
	note := recap.Note{
		Kind:     recap.KindRefuted,
		Body:     "只取分支统计未提交数会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go desktop/gitstats_cache.go",
		From:     path,
	}
	carry := func(priorTurns int, turn string) string {
		session := agent.NewSession("sys")
		for i := 0; i < priorTurns; i++ {
			session.Add(provider.Message{Role: provider.RoleUser, Content: "earlier turn"})
		}
		return New(Options{
			Executor:    agent.New(nil, nil, session, agent.Options{}, event.Discard),
			SessionDir:  filepath.Dir(path),
			SessionPath: path,
			Label:       "test",
			ProjectOffers: func(string) recap.Offers {
				return recap.Offers{Prior: []recap.Note{note}}
			},
		}).Compose(turn)
	}
	subject := "desktop/gitstats.go、desktop/gitstats_test.go、desktop/gitstats_cache.go 都要改"

	first := carry(0, subject)
	if !strings.Contains(first, "<prior-notes>") || !strings.Contains(first, "只取分支统计未提交数") {
		t.Fatalf("a first turn on the subject must carry the earlier conclusion: %q", first)
	}
	if strings.Contains(first, "<open-items>") {
		t.Fatalf("background must not read as an offer: %q", first)
	}
	if !strings.Contains(first, "as an instruction") || !strings.Contains(first, "what you observe wins") {
		t.Fatalf("background must stay checkable and subordinate to what the model sees: %q", first)
	}
	if !strings.Contains(first, "worked out the following") || !strings.Contains(first, "do not raise it as a question") {
		t.Fatalf("background must read as established project knowledge, not as unrequested noise: %q", first)
	}
	if !strings.HasSuffix(strings.TrimSpace(first), subject) {
		t.Fatalf("background must ride the turn tail, not replace it: %q", first)
	}

	// Mid-conversation the same turn carries it because it names the files.
	if mid := carry(3, subject); !strings.Contains(mid, "<prior-notes>") {
		t.Fatalf("a turn naming the note's own files must carry it mid-conversation: %q", mid)
	}
	// Shared wording is not enough there: a session deep in unrelated work shares
	// words with any note eventually.
	if prose := carry(3, "只取分支统计未提交数会漏掉 CJK 路径"); strings.Contains(prose, "<prior-notes>") {
		t.Fatalf("shared prose must not carry a conclusion mid-conversation: %q", prose)
	}
	// Nothing about the subject: silent, background or not.
	if other := carry(0, "帮我看看主题颜色为什么偏暗"); strings.Contains(other, "<prior-notes>") {
		t.Fatalf("an unrelated turn must stay silent: %q", other)
	}
}
