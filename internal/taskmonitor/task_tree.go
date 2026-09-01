package taskmonitor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// TaskTreeObserver records sub-agent work as task-tree rows (docs
// TASK_TREE_DESIGN.md §5.2): a group call (fleet/parallel_tasks) gets a
// synthesized parent row, each child run gets a row under it, and the parent
// carries persisted subtree aggregates. Implementations must be best-effort —
// monitoring must never fail the tool pipeline.
type TaskTreeObserver interface {
	// TreeTaskID resolves the monitor task id for a call id, so callers can
	// reference sibling rows in depends_on without duplicating the naming.
	TreeTaskID(sessionID, callID string) string
	// StartGroup creates the parent row for a fleet/parallel_tasks call and
	// returns its monitor task id. Callers pass the session and the tool call
	// id; the observer owns monitor-id namespacing.
	StartGroup(sessionID, callID, title string, now time.Time) string
	// FinishGroup advances the group row to its terminal state.
	FinishGroup(sessionID, callID string, state TaskState)
	// StartChild creates a row for one sub-agent run. parentID is empty for a
	// standalone task call (a root leaf).
	StartChild(sessionID, callID, parentID, title string, position int, dependsOn []string, now time.Time)
	// FinishChild advances a child row to its terminal state.
	FinishChild(sessionID, callID string, state TaskState)
	// RecordSkipped writes a skipped terminal row for a dependency-cut item.
	RecordSkipped(sessionID, callID, parentID, title string, position int, dependsOn []string, now time.Time)
}

// NewTaskTreeRecorder returns the store-backed TaskTreeObserver. It writes
// through a WriteStore — typically the taskcatalog ObservedStore — so rows
// project into SQLite automatically. The in-memory group aggregates are a
// per-process hint; the catalog recomputes them from children on the client.
func NewTaskTreeRecorder(store WriteStore, projectDir string) *TaskTreeRecorder {
	return &TaskTreeRecorder{
		store:      store,
		projectDir: projectDir,
		groups:     map[string]*treeGroupState{},
		childGroup: map[string]string{},
	}
}

// NewTaskTreeRecorderOrNil returns nil for a nil store so callers can wire the
// observer unconditionally; recording stays off without a task store.
func NewTaskTreeRecorderOrNil(store WriteStore, projectDir string) *TaskTreeRecorder {
	if store == nil {
		return nil
	}
	return NewTaskTreeRecorder(store, projectDir)
}

// TaskTreeRecorder is the store-backed TaskTreeObserver.
type TaskTreeRecorder struct {
	store      WriteStore
	projectDir string
	mu         sync.Mutex
	groups     map[string]*treeGroupState // group monitor id -> child aggregate
	childGroup map[string]string          // child monitor id -> group monitor id
}

type treeGroupState struct {
	done    int
	total   int
	failed  int
	skipped int
}

// treeTaskID namespaces a tool call id under its session, mirroring
// monitorTaskID so group rows, child rows, and jobs rows share one namespace.
// Call ids may contain "/" (fleet/parallel sub-ids); the catalog projection
// rejects slashes as path traversal, so they are re-encoded here.
func treeTaskID(sessionID, callID string) string {
	callID = strings.ReplaceAll(callID, "/", "--")
	if sessionID == "" {
		return sessionlessMonitorTaskID(callID)
	}
	return monitorTaskID(sessionID, callID)
}

func (r *TaskTreeRecorder) TreeTaskID(sessionID, callID string) string {
	return treeTaskID(sessionID, callID)
}

func (r *TaskTreeRecorder) StartGroup(sessionID, callID, title string, now time.Time) string {
	taskID := treeTaskID(sessionID, callID)
	r.mu.Lock()
	r.groups[taskID] = &treeGroupState{}
	r.mu.Unlock()
	r.saveTreeTask(taskID, sessionID, now, func(s *TaskSnapshot) {
		s.Title = title
		s.State = TaskStateRunning
	})
	return taskID
}

func (r *TaskTreeRecorder) FinishGroup(sessionID, callID string, state TaskState) {
	taskID := treeTaskID(sessionID, callID)
	r.mu.Lock()
	g := r.groups[taskID]
	delete(r.groups, taskID)
	r.mu.Unlock()
	if g == nil {
		return
	}
	r.update(taskID, func(s *TaskSnapshot) {
		s.State = state
		s.AggDone, s.AggTotal, s.AggFailed = g.done, g.total, g.failed
	})
}

func (r *TaskTreeRecorder) StartChild(sessionID, callID, parentID, title string, position int, dependsOn []string, now time.Time) {
	taskID := treeTaskID(sessionID, callID)
	if parentID != "" {
		r.mu.Lock()
		if g := r.groups[parentID]; g != nil {
			g.total++
		}
		r.childGroup[taskID] = parentID
		r.mu.Unlock()
	}
	r.saveTreeTask(taskID, sessionID, now, func(s *TaskSnapshot) {
		s.ParentID = parentID
		s.Position = position
		s.DependsOn = dependsOn
		s.Title = title
		s.State = TaskStateRunning
	})
}

func (r *TaskTreeRecorder) FinishChild(sessionID, callID string, state TaskState) {
	taskID := treeTaskID(sessionID, callID)
	r.mu.Lock()
	groupID := r.childGroup[taskID]
	if groupID != "" {
		if g := r.groups[groupID]; g != nil {
			switch state {
			case TaskStateSucceeded:
				g.done++
			case TaskStateFailed:
				g.failed++
			case TaskStateSkipped:
				g.skipped++
			}
		}
	}
	r.mu.Unlock()
	r.update(taskID, func(s *TaskSnapshot) {
		s.State = state
	})
	if groupID != "" {
		r.writeParentAgg(groupID)
	}
}

func (r *TaskTreeRecorder) RecordSkipped(sessionID, callID, parentID, title string, position int, dependsOn []string, now time.Time) {
	taskID := treeTaskID(sessionID, callID)
	if parentID != "" {
		r.mu.Lock()
		if g := r.groups[parentID]; g != nil {
			g.total++
			g.skipped++
		}
		r.childGroup[taskID] = parentID
		r.mu.Unlock()
		r.writeParentAgg(parentID)
	}
	r.saveTreeTask(taskID, sessionID, now, func(s *TaskSnapshot) {
		s.ParentID = parentID
		s.Position = position
		s.DependsOn = dependsOn
		s.Title = title
		s.State = TaskStateSkipped
	})
}

func (r *TaskTreeRecorder) writeParentAgg(groupID string) {
	r.mu.Lock()
	g := r.groups[groupID]
	r.mu.Unlock()
	if g == nil {
		return
	}
	r.update(groupID, func(s *TaskSnapshot) {
		s.AggDone, s.AggTotal, s.AggFailed = g.done, g.total, g.failed
	})
}

func (r *TaskTreeRecorder) saveTreeTask(taskID, sessionID string, now time.Time, mutate func(*TaskSnapshot)) {
	snap := TaskSnapshot{
		SchemaVersion: 2,
		TaskID:        taskID,
		SessionID:     sessionID,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	mutate(&snap)
	_ = r.store.SaveTask(context.Background(), r.projectDir, snap)
}

// update read-modifies-writes a row with version-CAS retry, mirroring
// TaskRecorder.RecordDone. Failures are swallowed: monitoring is best-effort.
func (r *TaskTreeRecorder) update(taskID string, mutate func(*TaskSnapshot)) {
	const maxAttempts = 4
	for range maxAttempts {
		cur, err := r.store.GetTask(context.Background(), r.projectDir, taskID)
		if err != nil || cur == nil {
			return
		}
		mutate(cur)
		cur.Version++
		cur.UpdatedAt = time.Now()
		if err := r.store.SaveTask(context.Background(), r.projectDir, *cur); err != nil {
			if errors.Is(err, ErrStoreVersionConflict) {
				continue
			}
			return
		}
		return
	}
}
