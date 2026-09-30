package main

import (
	"fmt"
	"strings"
	"time"

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

// CloseRecapHandoff marks one unfinished item handled.
func (a *App) CloseRecapHandoff(id string) error {
	ctx := a.bootContext()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.CloseOpen(ctx, id, time.Now())
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
