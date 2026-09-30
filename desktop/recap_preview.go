package main

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/boot"
	"reasonix/internal/recap"
)

// RecapPreviewView is what the page shows after a person presses "store to memory"
// or "draft a skill": the model's answer when it came, and — always — what that
// button does without a model, so a failed call costs a note rather than the action.
type RecapPreviewView struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Fallback  string `json:"fallback"`
	PromptTag string `json:"promptTag"`
	Model     string `json:"model"`
	Reason    string `json:"reason,omitempty"`
}

// recapPreviewCompose is the one call both previews make. It is a field so a test
// can answer without a provider, and the default is the real one.
type recapPreviewCompose func(ctx context.Context, kind, sessionPath, evidence string) (recap.ComposeResult, error)

func (a *App) composeForPreview(ctx context.Context, kind, sessionPath, evidence string) (recap.ComposeResult, error) {
	if a.recapPreview != nil {
		return a.recapPreview(ctx, kind, sessionPath, evidence)
	}
	if kind == "skill" {
		return boot.PreviewRecapSkill(ctx, sessionPath, evidence)
	}
	return boot.PreviewRecapMemory(ctx, sessionPath, evidence)
}

// PreviewRecapMemory asks the memory prompt to rewrite one note into the sentence
// that would be stored. The evidence carries the note's own pointers and tier plus
// the session's other notes as context, because a rewrite that cannot see the note's
// ground is guesswork.
func (a *App) PreviewRecapMemory(source RecapSkillSource) RecapPreviewView {
	view := RecapPreviewView{Kind: "memory"}
	record, entry, ok := a.recapNoteFor(source.Kind, source.Body)
	if !ok {
		view.Reason = "that note is no longer in the projection"
		return view
	}
	view.Fallback = oneLine(entry.Body)
	evidence := recapMemoryEvidence(record, entry)
	result, err := a.composeForPreview(a.bootContext(), "memory", record.Path, evidence)
	view.PromptTag, view.Model = result.PromptTag, result.Model
	switch {
	case err != nil:
		view.Reason = err.Error()
	case strings.TrimSpace(result.Text) == "":
		view.Reason = "the model returned nothing"
	default:
		view.Text = result.Text
	}
	if view.Reason == "" && result.Note != "" {
		view.Reason = result.Note
	}
	return view
}

// PreviewRecapSkill asks the playbook prompt to write one procedure from the notes
// the page grouped as a topic.
func (a *App) PreviewRecapSkill(sources []RecapSkillSource) RecapPreviewView {
	view := RecapPreviewView{Kind: "skill"}
	if len(sources) == 0 {
		view.Reason = "a draft needs at least one note"
		return view
	}
	entries := make([]recap.Entry, 0, len(sources))
	path := ""
	for _, source := range sources {
		record, entry, ok := a.recapNoteFor(strings.TrimSpace(source.Kind), strings.TrimSpace(source.Body))
		if !ok {
			view.Reason = "that note is no longer in the projection"
			return view
		}
		entries = append(entries, entry)
		if path == "" {
			path = record.Path
		}
	}
	// Without a model this button still composes something usable: the notes
	// verbatim, which is what it did before the prompt existed.
	view.Fallback = recapTopicSkillMarkdown("recap-"+stableSuggestionName(entries[0].Body, "note"), entries)
	result, err := a.composeForPreview(a.bootContext(), "skill", path, recapSkillEvidence(path, entries))
	view.PromptTag, view.Model = result.PromptTag, result.Model
	switch {
	case err != nil:
		view.Reason = err.Error()
	case strings.TrimSpace(result.Text) == "":
		view.Reason = "the model returned nothing"
	default:
		view.Text = result.Text
	}
	if view.Reason == "" && result.Note != "" {
		view.Reason = result.Note
	}
	return view
}

// recapNoteFor finds one note and the record it came from: the preview needs the
// session's path to resolve the same model the lane would.
func (a *App) recapNoteFor(kind, body string) (recap.Record, recap.Entry, bool) {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return recap.Record{}, recap.Entry{}, false
	}
	defer func() { _ = store.Close() }()
	records, err := store.List(ctx)
	if err != nil {
		return recap.Record{}, recap.Entry{}, false
	}
	for _, record := range records {
		for _, entry := range record.Entries {
			if entry.Kind == kind && entry.Body == body {
				return record, entry, true
			}
		}
	}
	return recap.Record{}, recap.Entry{}, false
}

// recapMemoryEvidence is the note plus what a rewrite needs to not guess: where it
// points, which tier it was proposed for, and the session's other notes as context.
func recapMemoryEvidence(record recap.Record, entry recap.Entry) string {
	var b strings.Builder
	b.WriteString("project: " + recap.ProjectOf(record.Path) + "\n")
	b.WriteString("note:\n")
	b.WriteString("  kind: " + entry.Kind + "\n")
	b.WriteString("  body: " + oneLine(entry.Body) + "\n")
	if evidence := oneLine(entry.Evidence); evidence != "" {
		b.WriteString("  evidence: " + evidence + "\n")
	}
	if refs := recapRefsLine(entry.Refs); refs != "" {
		b.WriteString("  pointers: " + refs + "\n")
	}
	if entry.Scope.Level != "" {
		b.WriteString(fmt.Sprintf("  proposed tier: %s (%s)\n", entry.Scope.Level, oneLine(entry.Scope.Reason)))
	}
	b.WriteString("neighbouring notes from the same session (context only, do not merge):\n")
	for _, other := range record.Entries {
		if other.Kind == entry.Kind && other.Body == entry.Body {
			continue
		}
		b.WriteString("- (" + other.Kind + ") " + oneLine(other.Body) + "\n")
	}
	return b.String()
}

// recapSkillEvidence lists the topic's notes in the order the session produced them,
// each with the pointers a step has to keep.
func recapSkillEvidence(sessionPath string, entries []recap.Entry) string {
	var b strings.Builder
	b.WriteString("project: " + recap.ProjectOf(sessionPath) + "\n")
	b.WriteString(fmt.Sprintf("notes on one topic, in the order the session produced them (%d):\n", len(entries)))
	for i, entry := range entries {
		b.WriteString(fmt.Sprintf("%d. (%s) %s\n", i+1, entry.Kind, oneLine(entry.Body)))
		if refs := recapRefsLine(entry.Refs); refs != "" {
			b.WriteString("   where to check: " + refs + "\n")
		}
		if evidence := oneLine(entry.Evidence); evidence != "" {
			b.WriteString("   evidence: " + evidence + "\n")
		}
	}
	return b.String()
}

func recapRefsLine(refs []recap.Ref) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		part := ref.Kind + " " + ref.Value
		if strings.TrimSpace(ref.Detail) != "" {
			part += " " + ref.Detail
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}
