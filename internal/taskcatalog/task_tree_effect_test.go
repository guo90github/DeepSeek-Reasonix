package taskcatalog

// Effect test for the task-tree persistence channel (docs/TASK_TREE_DESIGN.md
// §8 step 2): a fleet group with succeeded/failed/skipped children and a
// standalone leaf must land in the projection with agg_* and terminal states.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/taskmonitor"
)

func TestTaskTreeRecorderProjectsIntoCatalog(t *testing.T) {
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

	rec := taskmonitor.NewTaskTreeRecorder(catalog.ObservedStore(), projectRoot)
	now := time.Now()

	groupID := rec.StartGroup("session-1", "call-fleet", "fleet(2)", now)
	rec.StartChild("session-1", "call-fleet/fleet-1", groupID, "fleet-1", 1, nil, now)
	rec.StartChild("session-1", "call-fleet/fleet-2", groupID, "fleet-2", 2, []string{"dep-fleet-1"}, now)
	rec.FinishChild("session-1", "call-fleet/fleet-1", taskmonitor.TaskStateSucceeded)
	rec.FinishChild("session-1", "call-fleet/fleet-2", taskmonitor.TaskStateFailed)
	rec.RecordSkipped("session-1", "call-fleet/fleet-3", groupID, "fleet-3", 3, nil, now)
	rec.FinishGroup("session-1", "call-fleet", taskmonitor.TaskStateFailed)
	// Standalone task call: a root leaf with no parent.
	rec.StartChild("session-1", "call-task", "", "task", 0, nil, now)
	rec.FinishChild("session-1", "call-task", taskmonitor.TaskStateSucceeded)

	flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := catalog.Flush(flushCtx); err != nil {
		t.Fatal(err)
	}

	page, err := catalog.ListPage(ctx, PageRequest{ProjectKeys: []string{project.Key}, SessionID: "session-1", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	// The recorder derives ids as monitorTaskID(session, callID) with slashes
	// re-encoded as "--"; match rows by their full derived id.
	byID := map[string]taskmonitor.TaskSnapshot{}
	for _, item := range page.Items {
		byID[item.Task.TaskID] = item.Task
	}

	group, ok := byID["session-1--call-fleet"]
	if !ok {
		t.Fatalf("group row missing; items=%#v", page.Items)
	}
	if group.State != taskmonitor.TaskStateFailed || group.Title != "fleet(2)" {
		t.Fatalf("group state/title: %#v", group)
	}
	if group.AggDone != 1 || group.AggTotal != 3 || group.AggFailed != 1 {
		t.Fatalf("group agg = %d/%d failed=%d, want 1/3 failed=1", group.AggDone, group.AggTotal, group.AggFailed)
	}

	c1 := byID["session-1--call-fleet--fleet-1"]
	if c1.ParentID != groupID || c1.State != taskmonitor.TaskStateSucceeded || c1.Position != 1 {
		t.Fatalf("child 1: %#v", c1)
	}
	c2 := byID["session-1--call-fleet--fleet-2"]
	if c2.ParentID != groupID || c2.State != taskmonitor.TaskStateFailed || len(c2.DependsOn) != 1 || c2.DependsOn[0] != "dep-fleet-1" {
		t.Fatalf("child 2: %#v", c2)
	}
	c3 := byID["session-1--call-fleet--fleet-3"]
	if c3.ParentID != groupID || c3.State != taskmonitor.TaskStateSkipped || c3.Position != 3 {
		t.Fatalf("skipped child: %#v", c3)
	}
	leaf := byID["session-1--call-task"]
	if leaf.ParentID != "" || leaf.State != taskmonitor.TaskStateSucceeded {
		t.Fatalf("standalone leaf: %#v", leaf)
	}

	// The denormalized SQL columns must match the projection.
	var parentID string
	var aggDone, aggTotal, aggFailed int
	err = catalog.db.QueryRowContext(ctx,
		`SELECT parent_id, agg_done, agg_total, agg_failed FROM task_snapshots WHERE task_id=?`, group.TaskID).
		Scan(&parentID, &aggDone, &aggTotal, &aggFailed)
	if err != nil {
		t.Fatal(err)
	}
	if parentID != "" || aggDone != 1 || aggTotal != 3 || aggFailed != 1 {
		t.Fatalf("group row columns: parent=%q agg=%d/%d/%d", parentID, aggDone, aggTotal, aggFailed)
	}
}
