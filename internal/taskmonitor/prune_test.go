package taskmonitor

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func savePruneTask(t *testing.T, store WriteStore, project, id string, state TaskState, updated time.Time, version uint64) {
	t.Helper()
	snap := TaskSnapshot{SchemaVersion: 2, TaskID: id, SessionID: "s", Version: version,
		State: state, UpdatedAt: updated, CreatedAt: updated.Add(-time.Minute)}
	if state != TaskStateRunning {
		snap.RuntimeState = RuntimeStateExited
	}
	if err := store.SaveTask(context.Background(), project, snap); err != nil {
		t.Fatal(err)
	}
}

func archivePath(project, id string) string {
	return filepath.Join(project, ".reasonix", "tasks-archive", id)
}

func taskDirExists(project, id string) bool {
	_, err := os.Stat(filepath.Join(project, ".reasonix", "tasks", id))
	return err == nil
}

func TestPruneTasksArchivesOldestTerminalBeyondCap(t *testing.T) {
	project := t.TempDir()
	store := NewFileStore(".reasonix/tasks")
	now := time.Now()
	savePruneTask(t, store, project, "t1", TaskStateSucceeded, now.Add(-3*time.Hour), 1)
	savePruneTask(t, store, project, "t2", TaskStateSucceeded, now.Add(-2*time.Hour), 1)
	savePruneTask(t, store, project, "t3", TaskStateFailed, now.Add(-time.Hour), 1)
	savePruneTask(t, store, project, "t4", TaskStateRunning, now, 1)

	res, err := store.PruneTasks(context.Background(), project, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("Total must count terminal tasks only, got %d", res.Total)
	}
	if res.Archived != 1 {
		t.Fatalf("expected 1 archived (3 terminal - cap 2), got %d", res.Archived)
	}
	if _, err := os.Stat(archivePath(project, "t1")); err != nil {
		t.Fatalf("oldest terminal task t1 must be archived: %v", err)
	}
	for _, keep := range []string{"t2", "t3", "t4"} {
		if !taskDirExists(project, keep) {
			t.Fatalf("task %s must stay in the active tree", keep)
		}
	}
	if taskDirExists(project, "t1") {
		t.Fatal("archived task t1 must leave the active tree")
	}
}

func TestPruneTasksDefaultCapKeepsEverything(t *testing.T) {
	project := t.TempDir()
	store := NewFileStore(".reasonix/tasks")
	now := time.Now()
	for i := range 3 {
		savePruneTask(t, store, project, "t"+string(rune('a'+i)), TaskStateSucceeded, now.Add(-time.Duration(i)*time.Hour), 1)
	}
	res, err := store.PruneTasks(context.Background(), project, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Archived != 0 {
		t.Fatalf("default cap (500) must not archive a small tree, got %d", res.Archived)
	}
	if _, err := os.Stat(filepath.Join(project, ".reasonix", "tasks-archive")); !os.IsNotExist(err) {
		t.Fatalf("archive dir must not be created when nothing is archived, stat err=%v", err)
	}
}

func TestPruneTasksReplacesStaleArchiveCopy(t *testing.T) {
	project := t.TempDir()
	store := NewFileStore(".reasonix/tasks")
	now := time.Now()
	// t1 finishes, gets archived, then runs again (requeue) and finishes again.
	savePruneTask(t, store, project, "t1", TaskStateSucceeded, now.Add(-3*time.Hour), 1)
	savePruneTask(t, store, project, "t2", TaskStateSucceeded, now.Add(-2*time.Hour), 1)
	if _, err := store.PruneTasks(context.Background(), project, 1); err != nil {
		t.Fatal(err)
	}
	// Simulate the requeued second run with a marker inside the old archive.
	stale := archivePath(project, "t1")
	if err := os.WriteFile(filepath.Join(stale, "old-run-marker"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The requeued run keeps t1 the oldest terminal task so the next prune
	// targets it again and must replace the archived copy.
	savePruneTask(t, store, project, "t1", TaskStateFailed, now.Add(-3*time.Hour), 2)

	res, err := store.PruneTasks(context.Background(), project, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Archived != 1 {
		t.Fatalf("expected the newer t1 run to be archived, got %d", res.Archived)
	}
	if _, err := os.Stat(filepath.Join(stale, "old-run-marker")); !os.IsNotExist(err) {
		t.Fatal("stale archive copy must be replaced by the newer run")
	}
	if _, err := os.Stat(filepath.Join(stale, "snapshot.json")); err != nil {
		t.Fatalf("newer run must land in the archive: %v", err)
	}
}

func TestPruneTasksNeverTouchesRunningTasks(t *testing.T) {
	project := t.TempDir()
	store := NewFileStore(".reasonix/tasks")
	now := time.Now()
	savePruneTask(t, store, project, "run1", TaskStateRunning, now.Add(-time.Minute), 1)
	savePruneTask(t, store, project, "run2", TaskStateWaiting, now.Add(-30*time.Second), 1)
	savePruneTask(t, store, project, "done1", TaskStateSucceeded, now.Add(-2*time.Hour), 1)

	res, err := store.PruneTasks(context.Background(), project, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Archived != 0 {
		t.Fatalf("a lone terminal task under cap must not be archived, got %d", res.Archived)
	}
	for _, id := range []string{"run1", "run2", "done1"} {
		if !taskDirExists(project, id) {
			t.Fatalf("task %s must never be touched", id)
		}
	}
}

type recordingSink struct {
	mu    sync.Mutex
	hints map[string]int
}

func (s *recordingSink) SnapshotChanged(root, taskID string) {
	s.mu.Lock()
	s.hints[taskID]++
	s.mu.Unlock()
}
func (*recordingSink) EventsChanged(string, string) {}

func TestPruneTasksNotifiesProjectionSink(t *testing.T) {
	project := t.TempDir()
	sink := &recordingSink{hints: map[string]int{}}
	store := NewObservedFileStore(filepath.Join(".reasonix", "tasks"), sink)
	now := time.Now()
	savePruneTask(t, store, project, "t1", TaskStateSucceeded, now.Add(-2*time.Hour), 1)
	savePruneTask(t, store, project, "t2", TaskStateSucceeded, now.Add(-time.Hour), 1)
	before := map[string]int{}
	sink.mu.Lock()
	maps.Copy(before, sink.hints)
	sink.mu.Unlock()

	if _, err := store.PruneTasks(context.Background(), project, 1); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.hints["t1"] != before["t1"]+1 {
		t.Fatalf("archived task must hint the projection sink once, got %v (before %v)", sink.hints, before)
	}
	if sink.hints["t2"] != before["t2"] {
		t.Fatalf("retained task must not hint the sink, got %v (before %v)", sink.hints, before)
	}
}
