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

// RecapSkillSource names one note to draft from: the page sends the notes of one
// topic, and the host decides which of them the projection still holds.
type RecapSkillSource struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
}

// DraftRecapTopicSkill writes one playbook for a whole topic. A batch can honestly
// be one skill or several, and what decides it is the topic, not a count: notes the
// page grouped as one topic become one file, while notes of other topics are other
// calls and other files.
//
// A non-empty markdown is what the person reviewed and edited in the preview, and it
// is written as it stands. An empty one composes the notes verbatim — what this
// button did before a model was involved — so a failed preview leaves the action
// usable.
func (a *App) DraftRecapTopicSkill(sources []RecapSkillSource, markdown string) (RecapSkillDraft, error) {
	if len(sources) == 0 {
		return RecapSkillDraft{}, fmt.Errorf("a draft needs at least one note")
	}
	entries := make([]recap.Entry, 0, len(sources))
	for _, source := range sources {
		kind, body := strings.TrimSpace(source.Kind), strings.TrimSpace(source.Body)
		if !recapPlaybookKind(kind) {
			return RecapSkillDraft{}, fmt.Errorf("a %s note is not a playbook", kind)
		}
		entry, ok := a.findRecapEntry(kind, body)
		if !ok {
			return RecapSkillDraft{}, fmt.Errorf("that note is no longer in the projection")
		}
		entries = append(entries, entry)
	}
	root := a.projectRootForDrafting()
	if root == "" {
		return RecapSkillDraft{}, fmt.Errorf("no project is open to draft into")
	}
	name := "recap-" + stableSuggestionName(entries[0].Body, "note")
	dir := filepath.Join(root, ".reasonix", skill.SkillsDirname, name)
	target := filepath.Join(dir, skill.SkillFile)
	if _, err := os.Stat(target); err == nil {
		return RecapSkillDraft{}, fmt.Errorf("%s already exists; rename or remove it first", target)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RecapSkillDraft{}, err
	}
	body := strings.TrimSpace(markdown)
	if body == "" {
		body = recapTopicSkillMarkdown(name, entries)
	} else {
		body = withRecapSkillPreamble(name, body)
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		return RecapSkillDraft{}, err
	}
	return RecapSkillDraft{Name: name, Path: target}, nil
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

// withRecapSkillPreamble gives a reviewed playbook the frontmatter a skill needs.
// The reviewed body is kept as it stands: this path never rewrites what a person read
// and approved.
func withRecapSkillPreamble(name, body string) string {
	if strings.HasPrefix(strings.TrimSpace(body), "---") {
		return body
	}
	description := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(strings.TrimLeft(line, "# "))
		if trimmed != "" {
			description = trimmed
			break
		}
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	b.WriteString("description: " + quoteYAMLScalar(oneLine(description)) + "\n")
	b.WriteString("invocation: manual\n")
	b.WriteString("---\n\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

// recapTopicSkillMarkdown keeps a single-note draft byte-identical to what the
// per-note action has always written, and lays several notes of one topic out in
// the order the session produced them. Every step is a note verbatim with its own
// pointers: composition here is juxtaposition, never rewriting.
// recapTopicSkillMarkdown keeps a single-note draft byte-identical to what the
// per-note action has always written, and lays several notes of one topic out in
// the order the session produced them. Every step is a note verbatim with its own
// pointers: composition here is juxtaposition, never rewriting.
func recapTopicSkillMarkdown(name string, entries []recap.Entry) string {
	if len(entries) == 1 {
		return recapSkillMarkdown(name, entries[0])
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	b.WriteString("description: " + quoteYAMLScalar(fmt.Sprintf("%s (and %d more notes on the same topic)",
		oneLine(entries[0].Body), len(entries)-1)) + "\n")
	b.WriteString("invocation: manual\n")
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("> Draft distilled from %d session recap notes on one topic. Each step\n", len(entries)))
	b.WriteString("> below is a note verbatim, with its own pointers — nothing was rewritten or\n")
	b.WriteString("> inferred. Read it, verify the steps, and change `invocation: manual` to\n")
	b.WriteString("> `auto` only once you trust it.\n\n")
	b.WriteString("## When it applies\n\n")
	for _, entry := range entries {
		b.WriteString("- " + oneLine(entry.Body) + "\n")
	}
	b.WriteString("\n## What to do\n\n")
	for i, entry := range entries {
		b.WriteString(fmt.Sprintf("%d. (%s) %s\n", i+1, entry.Kind, oneLine(entry.Body)))
		if len(entry.Refs) > 0 {
			lines := make([]string, 0, len(entry.Refs))
			for _, ref := range entry.Refs {
				lines = append(lines, refLine(ref))
			}
			b.WriteString("   Where to check: " + strings.Join(lines, "; ") + "\n")
		}
		if evidence := oneLine(entry.Evidence); evidence != "" {
			b.WriteString("   Evidence: " + evidence + "\n")
		}
	}
	return b.String()
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
