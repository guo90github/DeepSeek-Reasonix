package main

import (
	"strings"
	"testing"

	"reasonix/internal/frontmatter"
	"reasonix/internal/recap"
)

// The draft has to be a *valid* skill, or it is a file nobody can invoke: its
// frontmatter must parse, it must carry a description the catalog can show, and it
// must be manual so an unread draft cannot be reached for on its own.
func TestRecapSkillDraftIsAValidManualSkill(t *testing.T) {
	entry := recap.Entry{
		Kind:     recap.KindRootCause,
		Body:     "只取分支统计未提交数：漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go",
		Refs:     []recap.Ref{{Kind: recap.RefPath, Value: "desktop/gitstats.go", Detail: "L40"}},
	}
	markdown := recapSkillMarkdown("recap-note-1a2b3c4d", entry)
	fields, body := frontmatter.Split(markdown)
	if fields["name"] != "recap-note-1a2b3c4d" || fields["invocation"] != "manual" {
		t.Fatalf("frontmatter = %+v, want the directory's name and manual invocation", fields)
	}
	if !strings.Contains(fields["description"], "只取分支统计未提交数") {
		t.Fatalf("the description must survive the colon in the note: %q", fields["description"])
	}
	if !strings.Contains(body, "Where to check") || !strings.Contains(body, "path desktop/gitstats.go L40") {
		t.Fatalf("the draft must carry the pointers: %q", body)
	}
	if !strings.Contains(body, "invocation: manual") {
		t.Fatalf("the draft must tell the person how to arm it: %q", body)
	}
}

// A draft nobody asked for is the failure mode this whole channel avoids, so the
// kinds that are not procedures never reach the writer.
func TestRecapPlaybookKindKeepsOnlyProcedures(t *testing.T) {
	if !recapPlaybookKind(recap.KindRootCause) || !recapPlaybookKind(recap.KindRefuted) {
		t.Fatal("a diagnosis and a ruled-out approach are procedures")
	}
	if recapPlaybookKind(recap.KindFact) || recapPlaybookKind(recap.KindHandoff) {
		t.Fatal("a fact is not a procedure and an unfinished item is a reminder")
	}
}

func TestQuoteYAMLScalarKeepsOneValue(t *testing.T) {
	got := quoteYAMLScalar(`he said "check: it" then stopped`)
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("a scalar must come back quoted: %q", got)
	}
	if strings.Count(got, `\"`) != 2 || !strings.Contains(got, `\:`) && !strings.Contains(got, `:`) {
		t.Fatalf("quotes inside the value must be escaped: %q", got)
	}
	fields, _ := frontmatter.Split("---\ndescription: " + got + "\n---\nbody\n")
	if strings.TrimSpace(fields["description"]) == "" {
		t.Fatalf("a value with a colon and quotes must still parse: %+v", fields)
	}
}
