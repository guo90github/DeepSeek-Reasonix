package taskcatalog

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/taskmonitor"
)

func snapshot(id, session string, version uint64, updated time.Time) taskmonitor.TaskSnapshot {
	return taskmonitor.TaskSnapshot{SchemaVersion: 1, TaskID: id, SessionID: session, State: taskmonitor.TaskStateRunning,
		RuntimeState: taskmonitor.RuntimeStateAlive, RuntimeLeaseUntil: updated.Add(time.Hour), Version: version,
		CreatedAt: updated.Add(-time.Minute), UpdatedAt: updated}
}

func TestObservedStoreIndexesSnapshotsAndEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projectRoot := t.TempDir()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	project, err := catalog.RegisterProject(ctx, projectRoot, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.ObservedStore()
	now := time.Now()
	if err := store.SaveTask(ctx, projectRoot, snapshot("task-1", "session-1", 1, now)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendAuditEvent(ctx, projectRoot, taskmonitor.TaskEvent{Timestamp: now, EventType: "state_change", TaskID: "task-1", SessionID: "session-1", State: taskmonitor.TaskStateRunning}); err != nil {
		t.Fatal(err)
	}
	flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := catalog.Flush(flushCtx); err != nil {
		t.Fatal(err)
	}
	page, err := catalog.ListPage(ctx, PageRequest{ProjectKeys: []string{project.Key}, SessionID: "session-1", Limit: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].Task.TaskID != "task-1" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	events, err := catalog.ListEventPage(ctx, project.Key, "task-1", 0, 50)
	if err != nil || len(events.Items) != 1 || events.NextSequence != 1 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if err := store.AppendAuditEvent(ctx, projectRoot, taskmonitor.TaskEvent{Timestamp: now.Add(time.Second), EventType: "state_change",
		TaskID: "task-1", SessionID: "session-1", State: taskmonitor.TaskStateSucceeded}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Flush(flushCtx); err != nil {
		t.Fatal(err)
	}
	events, err = catalog.ListEventPage(ctx, project.Key, "task-1", 1, 50)
	if err != nil || len(events.Items) != 1 || events.Items[0].Sequence != 2 {
		t.Fatalf("incremental events=%#v err=%v", events, err)
	}
}

func TestPageCursorIsRevisionBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	project, _ := catalog.RegisterProject(ctx, root, "Demo")
	store := catalog.ObservedStore()
	now := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		if err := store.SaveTask(ctx, root, snapshot(id, "session", 1, now.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = catalog.Flush(flushCtx)
	first, err := catalog.ListPage(ctx, PageRequest{ProjectKeys: []string{project.Key}, Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	if err := store.SaveTask(ctx, root, snapshot("d", "session", 1, now.Add(4*time.Minute))); err != nil {
		t.Fatal(err)
	}
	_ = catalog.Flush(flushCtx)
	stale, err := catalog.ListPage(ctx, PageRequest{ProjectKeys: []string{project.Key}, Cursor: first.NextCursor})
	if err != nil || !stale.StaleCursor || len(stale.Items) != 0 {
		t.Fatalf("stale=%#v err=%v", stale, err)
	}
}

func TestListPageSurfacesTombstoneAsRemoved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projectRoot := t.TempDir()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	project, err := catalog.RegisterProject(ctx, projectRoot, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.ObservedStore()
	now := time.Now()
	if err := store.SaveTask(ctx, projectRoot, snapshot("task-1", "session-1", 1, now)); err != nil {
		t.Fatal(err)
	}
	flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := catalog.Flush(flushCtx); err != nil {
		t.Fatal(err)
	}

	// Healthy row: page returns it with its stored state.
	page, err := catalog.ListPage(ctx, PageRequest{ProjectKeys: []string{project.Key}, Limit: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].Task.State != taskmonitor.TaskStateRunning {
		t.Fatalf("healthy page=%#v err=%v", page, err)
	}

	// Tombstone the row (as reconcile does when the task file disappears).
	if _, err := catalog.db.ExecContext(ctx, `UPDATE task_snapshots SET health='missing', missing_since=? WHERE project_key=? AND task_id=?`,
		now.UnixMilli(), project.Key, "task-1"); err != nil {
		t.Fatal(err)
	}

	// During the grace window the page must surface the row as removed.
	page, err = catalog.ListPage(ctx, PageRequest{ProjectKeys: []string{project.Key}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("tombstone row must stay visible during its grace window, got %d items", len(page.Items))
	}
	if page.Items[0].Task.State != taskmonitor.TaskStateRemoved {
		t.Fatalf("tombstone state = %q, want removed", page.Items[0].Task.State)
	}
}
