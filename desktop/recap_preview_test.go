package main

import (
	"strings"
	"testing"

	"reasonix/internal/recap"
)

// A rewrite that cannot see the note's ground is guesswork, so the evidence carries
// the note's own pointers and tier plus the session's other notes as context — and
// never the note twice, which would read as emphasis.
func TestRecapMemoryEvidenceCarriesTheNoteAndItsContext(t *testing.T) {
	record := recap.Record{Path: `C:\state\projects\c--guosj-ai-demo\sessions\s.jsonl`, Entries: []recap.Entry{
		{Kind: recap.KindRootCause, Body: "外层 SELECT 拿不到 ID", Evidence: "internal/parser.go",
			Refs:  []recap.Ref{{Kind: recap.RefPath, Value: "internal/parser.go", Detail: "L40"}},
			Scope: recap.Scope{Level: recap.ScopeProject, Reason: "只在这个仓库为真"}},
		{Kind: recap.KindHandoff, Body: "还没验 verify 脚本"},
	}}
	evidence := recapMemoryEvidence(record, record.Entries[0])
	for _, want := range []string{
		// ProjectOf answers with the project directory, not a slug: the preview shows
		// the same path the lane would key on.
		"project: ",
		"c--guosj-ai-demo",
		"kind: root-cause",
		"body: 外层 SELECT 拿不到 ID",
		"evidence: internal/parser.go",
		"pointers: path internal/parser.go L40",
		"proposed tier: project (只在这个仓库为真)",
		"neighbouring notes from the same session (context only, do not merge):",
		"- (handoff) 还没验 verify 脚本",
	} {
		if !strings.Contains(evidence, want) {
			t.Fatalf("memory evidence must carry %q:\n%s", want, evidence)
		}
	}
	if strings.Count(evidence, "外层 SELECT 拿不到 ID") != 1 {
		t.Fatalf("the note must not also appear as its own neighbour:\n%s", evidence)
	}
}

// The playbook needs the notes in the order the session produced them, each with the
// pointers its step has to keep.
func TestRecapSkillEvidenceKeepsOrderAndPointers(t *testing.T) {
	entries := []recap.Entry{
		{Kind: recap.KindRootCause, Body: "只取分支统计未提交数",
			Refs: []recap.Ref{{Kind: recap.RefPath, Value: "desktop/gitstats.go", Detail: "L40"}}},
		{Kind: recap.KindRefuted, Body: "不要再按文件名猜语言", Evidence: "git blame",
			Refs: []recap.Ref{{Kind: recap.RefCommand, Value: "git blame -L 12,12"}}},
	}
	evidence := recapSkillEvidence(`C:\state\projects\c--guosj-ai-demo\sessions\s.jsonl`, entries)
	if !strings.Contains(evidence, "project: ") || !strings.Contains(evidence, "c--guosj-ai-demo") {
		// ProjectOf answers with the project directory, not a slug.
		t.Fatalf("skill evidence must name the project:\n%s", evidence)
	}
	if !strings.Contains(evidence, "1. (root-cause)") || !strings.Contains(evidence, "2. (refuted)") {
		t.Fatalf("the notes must stay in order and carry their kind:\n%s", evidence)
	}
	if !strings.Contains(evidence, "where to check: path desktop/gitstats.go L40") ||
		!strings.Contains(evidence, "where to check: command git blame -L 12,12") {
		t.Fatalf("each note must carry its own pointers:\n%s", evidence)
	}
}

// A preview that cannot find its note says so instead of asking a model about
// nothing: the page then offers only what it can still do.
func TestPreviewWithoutANoteExplainsItself(t *testing.T) {
	isolateDesktopUserDirs(t)
	view := NewApp().PreviewRecapMemory(RecapSkillSource{Kind: recap.KindFact, Body: "一条不在投影里的条目"})
	if view.Kind != "memory" || view.Text != "" || view.Reason == "" {
		t.Fatalf("preview = %+v, want no text and a stated reason", view)
	}
	empty := NewApp().PreviewRecapSkill(nil)
	if empty.Reason == "" {
		t.Fatalf("an empty batch must say why: %+v", empty)
	}
}
