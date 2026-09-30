package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/recap"
	"reasonix/internal/skill"
)

// RecapSkillDraft says where a playbook draft landed.
type RecapSkillDraft struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// DraftRecapSkill writes a playbook from one reviewed note into this project's
// skill directory. Three things are deliberate: it is a person-triggered action,
// it never overwrites a file that is already there, and the draft declares
// `invocation: manual` — so nothing the model can reach for on its own came out of
// a note nobody has read yet.
func (a *App) DraftRecapSkill(kind, body string) (RecapSkillDraft, error) {
	kind, body = strings.TrimSpace(kind), strings.TrimSpace(body)
	if body == "" {
		return RecapSkillDraft{}, fmt.Errorf("a note needs a body")
	}
	if !recapPlaybookKind(kind) {
		return RecapSkillDraft{}, fmt.Errorf("a %s note is not a playbook", kind)
	}
	root := a.projectRootForDrafting()
	if root == "" {
		return RecapSkillDraft{}, fmt.Errorf("no project is open to draft into")
	}
	entry, ok := a.findRecapEntry(kind, body)
	if !ok {
		return RecapSkillDraft{}, fmt.Errorf("that note is no longer in the projection")
	}
	name := "recap-" + stableSuggestionName(body, "note")
	dir := filepath.Join(root, ".reasonix", skill.SkillsDirname, name)
	target := filepath.Join(dir, skill.SkillFile)
	if _, err := os.Stat(target); err == nil {
		return RecapSkillDraft{}, fmt.Errorf("%s already exists; rename or remove it first", target)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RecapSkillDraft{}, err
	}
	if err := os.WriteFile(target, []byte(recapSkillMarkdown(name, entry)), 0o644); err != nil {
		return RecapSkillDraft{}, err
	}
	return RecapSkillDraft{Name: name, Path: target}, nil
}

// recapPlaybookKind keeps the draft honest: a diagnosis and a ruled-out approach
// are procedures, while a fact is not one and an unfinished item is a reminder.
func recapPlaybookKind(kind string) bool {
	return kind == recap.KindRootCause || kind == recap.KindRefuted
}

// projectRootForDrafting is the project the open tab works in. Unlike
// activeWorkspaceRoot it never falls back to ".": a playbook drafted into the
// process's own directory is a playbook nobody will find, so a global tab means no
// draft rather than a surprising one.
func (a *App) projectRootForDrafting() string {
	tab := a.activeTab()
	if tab == nil {
		return ""
	}
	a.reconcileTabWithPinnedSessionMeta(tab)
	return strings.TrimSpace(tab.WorkspaceRoot)
}

// recapSkillMarkdown is the draft itself: what it applies to, what to do, and where
// to check. It stays short on purpose — a playbook nobody reads is a playbook
// nobody edits.
func recapSkillMarkdown(name string, entry recap.Entry) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	b.WriteString("description: " + quoteYAMLScalar(oneLine(entry.Body)) + "\n")
	b.WriteString("invocation: manual\n")
	b.WriteString("---\n\n")
	b.WriteString("> Draft distilled from a session recap. Read it, fix it, and change\n")
	b.WriteString("> `invocation: manual` to `auto` only once you trust it.\n\n")
	b.WriteString("## When it applies\n\n" + oneLine(entry.Body) + "\n\n")
	b.WriteString("## What to do\n\n")
	switch entry.Kind {
	case recap.KindRootCause:
		b.WriteString("Check the diagnosis before acting on it, then fix the cause rather than the symptom.\n")
	case recap.KindRefuted:
		b.WriteString("Do not propose this again without new evidence; if you are about to, say what changed.\n")
	}
	if len(entry.Refs) > 0 {
		b.WriteString("\n## Where to check\n\n")
		for _, ref := range entry.Refs {
			b.WriteString("- " + refLine(ref) + "\n")
		}
	}
	if evidence := oneLine(entry.Evidence); evidence != "" {
		b.WriteString("\n## Where this came from\n\n" + evidence + "\n")
	}
	return b.String()
}

// quoteYAMLScalar keeps a value on one line and inside its quotes: an unquoted
// colon in a note would otherwise break the frontmatter, and a draft whose
// description does not parse is a draft nobody can find.
func quoteYAMLScalar(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
