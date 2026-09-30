package main

import (
	"fmt"
	"strings"
	"time"

	"reasonix/internal/memory"
	"reasonix/internal/recap"
)

// AcceptRecapEntry saves one reviewed note as an active memory fact and records
// the choice, returning the memory name it was saved under. Only experience notes
// can be accepted: a handoff note is a reminder for whoever reads the page, so it
// has nowhere to be stored.
func (a *App) AcceptRecapEntry(kind, body, editedBody string) (string, error) {
	entry := recap.Entry{Kind: strings.TrimSpace(kind), Body: strings.TrimSpace(body)}
	if entry.Body == "" {
		return "", fmt.Errorf("a note needs a body")
	}
	if recap.Sink(entry.Kind) != recap.SinkMemory {
		return "", fmt.Errorf("a %s note is shown only, so it cannot be accepted", entry.Kind)
	}
	// Accepting writes what the note actually cited. The page carries only kind and
	// body, so the note is read back from the projection: the pointers and the tier
	// it proposed live there, not in the request.
	if found, ok := a.findRecapEntry(entry.Kind, entry.Body); ok {
		entry = found
	}
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return "", fmt.Errorf("no open session to write memory into")
	}
	text := strings.TrimSpace(editedBody)
	if text == "" {
		text = entry.Body
	}
	name, err := ctrl.SaveMemory(recapMemoryFact(entry, text))
	if err != nil {
		return "", err
	}
	if err := a.decideRecapEntry(entry, recap.DecisionAccept); err != nil {
		return name, err
	}
	return name, nil
}

// RejectRecapEntry remembers that one note was ruled out, so recapping the same
// session never offers it again.
func (a *App) RejectRecapEntry(kind, body string) error {
	entry := recap.Entry{Kind: strings.TrimSpace(kind), Body: strings.TrimSpace(body)}
	if entry.Body == "" {
		return fmt.Errorf("a note needs a body")
	}
	return a.decideRecapEntry(entry, recap.DecisionReject)
}

// UndoRecapEntry drops a note's recorded choice: the way back from a mis-click on
// 弃. It never touches memory — an accepted fact is deleted on the memory page.
func (a *App) UndoRecapEntry(kind, body string) error {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.ClearDecision(ctx, strings.TrimSpace(kind), strings.TrimSpace(body))
}

// findRecapEntry reads one note back out of the projection by its identity, which
// is what a note is keyed by everywhere else. Accepting uses it so the memory fact
// can carry the pointers the note cited, which the page never sends.
func (a *App) findRecapEntry(kind, body string) (recap.Entry, bool) {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return recap.Entry{}, false
	}
	defer func() { _ = store.Close() }()
	records, err := store.List(ctx)
	if err != nil {
		return recap.Entry{}, false
	}
	for _, rec := range records {
		for _, entry := range rec.Entries {
			if entry.Kind == kind && entry.Body == body {
				return entry, true
			}
		}
	}
	return recap.Entry{}, false
}

// decideRecapEntry opens the projection for one write, the same way every other
// caller in this file does: no long-lived handle.
func (a *App) decideRecapEntry(entry recap.Entry, choice string) error {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.Decide(ctx, entry.Kind, entry.Body, choice, time.Now())
}

// recapMemoryFact turns a reviewed note into the memory fact it becomes: the
// note's text is the fact, its kind decides the type, and its provenance is kept
// as evidence rather than folded into the claim.
func recapMemoryFact(entry recap.Entry, text string) memory.Memory {
	return memory.Memory{
		Name:        stableSuggestionName(text, "recap"),
		Title:       suggestionTitle(text, "Recap note"),
		Description: oneLine(text),
		Type:        recapMemoryType(entry.Kind),
		Scope:       memory.FactScopeProject,
		Body:        recapMemoryBody(entry, text),
	}
}

// recapMemoryType maps a note's kind onto the memory type that carries it: a
// ruled-out option is guidance about how to work, the rest are project facts.
func recapMemoryType(kind string) memory.Type {
	if kind == recap.KindRefuted {
		return memory.TypeFeedback
	}
	return memory.TypeProject
}

func recapMemoryBody(entry recap.Entry, text string) string {
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n**Why:** ")
	switch entry.Kind {
	case recap.KindRootCause:
		b.WriteString("a root cause diagnosed in a closed session.")
	case recap.KindRefuted:
		b.WriteString("ruled out earlier; do not propose it again without new evidence.")
	default:
		b.WriteString("distilled from a closed session.")
	}
	b.WriteString("\n**How to apply:** Check it against the code before relying on it.\n")
	if evidence := oneLine(entry.Evidence); evidence != "" {
		b.WriteString("\nEvidence: " + evidence + "\n")
	}
	if len(entry.Refs) > 0 {
		b.WriteString("\n**Pointers:**\n")
		for _, ref := range entry.Refs {
			b.WriteString("- " + refLine(ref) + "\n")
		}
	}
	// The tier is reported, never applied: memory is written on an explicit accept
	// and every accept still lands in the current project, so a tier that says
	// "in general" cannot quietly move a fact out of it.
	if entry.Scope.Level != "" {
		b.WriteString("\n**Scope proposed:** " + entry.Scope.Level)
		if reason := oneLine(entry.Scope.Reason); reason != "" {
			b.WriteString(" — " + reason)
		}
		b.WriteString(" (accepting still writes to the current project)\n")
	}
	return b.String()
}

// refLine renders one pointer as a single line: where, and what part of it.
func refLine(ref recap.Ref) string {
	line := strings.TrimSpace(ref.Kind + " " + ref.Value)
	if detail := oneLine(ref.Detail); detail != "" {
		line += " " + detail
	}
	return line
}
