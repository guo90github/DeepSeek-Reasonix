package boot

import (
	"context"
	"strings"

	"reasonix/internal/recap"
)

// recapOpenHandoffs reads one project's unfinished items for the controller's
// turn-tail offer. The projection is opened per call: an offer only happens on a
// session's first turn or on a turn that says it continues something, so a
// cached handle would buy little and hold the file open between them.
func recapOpenHandoffs(project string) []recap.OpenItem {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil
	}
	ctx := context.Background()
	store, err := recap.Open(ctx, recap.Options{Path: recap.DefaultPath()})
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	items, err := store.OpenItemsFor(ctx, project)
	if err != nil {
		return nil
	}
	return items
}
