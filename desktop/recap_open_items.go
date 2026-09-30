package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"reasonix/internal/memory"
	"reasonix/internal/recap"
)

// RecapOpenItemView is one unfinished item as the recap page consumes it.
type RecapOpenItemView struct {
	ID       string `json:"id"`
	Body     string `json:"body"`
	Evidence string `json:"evidence,omitempty"`
	From     string `json:"from,omitempty"`
	OpenedAt string `json:"openedAt"`
	Closed   bool   `json:"closed"`
	// Stale says the item has aged past automatic offers while staying on this
	// list. It is computed here so the page never restates the window.
	Stale   bool `json:"stale,omitempty"`
	AgeDays int  `json:"ageDays"`
}

// KeepRecapHandoff keeps one handoff note as an unfinished item of the session's
// project, so a later session that continues the work can be offered it. The item
// stays open until someone closes it: the session that continues the work is
// often not the next one.
func (a *App) KeepRecapHandoff(sessionPath, body, evidence string) (string, error) {
	text := strings.TrimSpace(body)
	if text == "" {
		return "", fmt.Errorf("an unfinished item needs a body")
	}
	project := recap.ProjectOf(sessionPath)
	if project == "" {
		return "", fmt.Errorf("the session does not name a project")
	}
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return "", err
	}
	defer func() { _ = store.Close() }()
	if err := store.KeepOpen(ctx, recap.OpenItem{
		Project:  project,
		Body:     text,
		Evidence: strings.TrimSpace(evidence),
		From:     strings.TrimSpace(sessionPath),
	}, time.Now()); err != nil {
		return "", err
	}
	return recap.HashEntry(recap.KindHandoff, text), nil
}

// CloseRecapHandoff marks one unfinished item handled. A one-line outcome, when
// the person writes one, becomes sediment instead of dying with the item: it is
// their own statement about how the work ended, so it lands in memory — the layer
// the next session actually reads — and needs no model to get there.
func (a *App) CloseRecapHandoff(id, body, evidence, resolution string) error {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if err := store.CloseOpen(ctx, id, time.Now()); err != nil {
		return err
	}
	if text := strings.TrimSpace(resolution); text != "" {
		a.recordRecapOutcome(body, evidence, text)
	}
	return nil
}

// recordRecapOutcome writes a resolved item's outcome into the project's memory.
// Best effort: the item is already closed, and losing the outcome must not undo
// that. No session open means nowhere to write it, which is not an error.
func (a *App) recordRecapOutcome(body, evidence, resolution string) {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return
	}
	if _, err := ctrl.SaveMemory(recapOutcomeFact(body, evidence, resolution)); err != nil {
		slog.Warn("desktop: could not record a resolved item's outcome", "err", err)
	}
}

// recapOutcomeFact is the outcome itself plus what it is answering: a resolution
// without the item reads like a rule with no question behind it.
func recapOutcomeFact(body, evidence, resolution string) memory.Memory {
	resolution = strings.TrimSpace(resolution)
	fact := memory.Memory{
		Name:        stableSuggestionName(resolution, "recap-outcome"),
		Title:       suggestionTitle(resolution, "Resolved item"),
		Description: oneLine(resolution),
		Type:        memory.TypeProject,
		Scope:       memory.FactScopeProject,
		Body: resolution + `

**Why:** An unfinished item from an earlier session ended this way.
**How to apply:** Check it against the code before relying on it.
`,
	}
	if item := oneLine(body); item != "" {
		fact.Body += "\n**Item:** " + item + "\n"
	}
	if quote := oneLine(evidence); quote != "" {
		fact.Body += "\nEvidence: " + quote + "\n"
	}
	return fact
}

// ReopenRecapHandoff puts a handled item back on the list, for a mis-click.
func (a *App) ReopenRecapHandoff(id string) error {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.ReopenOpen(ctx, id)
}

// ListRecapOpenItems returns the current project's unfinished items, open ones
// first. It is the list that keeps "nothing was offered this session" from being
// invisible: a kept item is always somewhere the person can see it.
func (a *App) ListRecapOpenItems() []RecapOpenItemView {
	project := recap.ProjectOfDir(a.activeSessionDir())
	if project == "" {
		return []RecapOpenItemView{}
	}
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return []RecapOpenItemView{}
	}
	defer func() { _ = store.Close() }()
	items, err := store.OpenItemsForProject(ctx, project)
	if err != nil {
		return []RecapOpenItemView{}
	}
	out := make([]RecapOpenItemView, 0, len(items))
	now := time.Now()
	for _, item := range items {
		out = append(out, RecapOpenItemView{
			ID:       item.ID,
			Body:     item.Body,
			Evidence: item.Evidence,
			From:     item.From,
			OpenedAt: item.OpenedAt.Format(time.RFC3339),
			Closed:   !item.Open(),
			Stale:    item.Open() && item.TooOld(now),
			AgeDays:  int(now.Sub(item.OpenedAt).Hours() / 24),
		})
	}
	return out
}
