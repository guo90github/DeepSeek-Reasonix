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
	return b.String()
}
