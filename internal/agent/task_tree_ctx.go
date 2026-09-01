package agent

// Task-tree recording context (docs/TASK_TREE_DESIGN.md §5.2.2): group calls
// stamp their observer and base slot; children override the slot per item.

import (
	"context"
	"fmt"
	"time"

	"reasonix/internal/taskmonitor"
)

// taskTreeSlot carries per-call recording state. groupID empty means a
// standalone task call (a root leaf).
type taskTreeSlot struct {
	groupID   string
	position  int
	dependsOn []string
}

type taskTreeSlotKey struct{}

func withTaskTreeSlot(ctx context.Context, s taskTreeSlot) context.Context {
	return context.WithValue(ctx, taskTreeSlotKey{}, s)
}

func taskTreeSlotFromContext(ctx context.Context) (taskTreeSlot, bool) {
	s, ok := ctx.Value(taskTreeSlotKey{}).(taskTreeSlot)
	return s, ok
}

type taskTreeObserverKey struct{}

func withTaskTreeObserver(ctx context.Context, obs taskmonitor.TaskTreeObserver) context.Context {
	if obs == nil {
		return ctx
	}
	return context.WithValue(ctx, taskTreeObserverKey{}, obs)
}

func taskTreeObserverFromContext(ctx context.Context) taskmonitor.TaskTreeObserver {
	obs, _ := ctx.Value(taskTreeObserverKey{}).(taskmonitor.TaskTreeObserver)
	return obs
}

// Background fleet/parallel runs suppress tree rows: the jobs recorder writes
// the job row, and per-item rows would be orphaned roots.
type taskTreeSuppressKey struct{}

func withTaskTreeSuppress(ctx context.Context) context.Context {
	return context.WithValue(ctx, taskTreeSuppressKey{}, true)
}

func taskTreeSuppressed(ctx context.Context) bool {
	v, _ := ctx.Value(taskTreeSuppressKey{}).(bool)
	return v
}

// WithTreeObserver installs the task-tree recorder for sub-agent rows (docs
// TASK_TREE_DESIGN.md §5.2). nil disables tree recording.
func (t *TaskTool) WithTreeObserver(obs taskmonitor.TaskTreeObserver) *TaskTool {
	if t == nil {
		return nil
	}
	t.treeObserver = obs
	return t
}

// startTreeGroup creates the group row and stamps observer + base slot. The
// returned group id is empty when no observer is installed.
func startTreeGroup(ctx context.Context, obs taskmonitor.TaskTreeObserver, sessionID, callID, title string) (context.Context, string) {
	if obs == nil {
		return ctx, ""
	}
	groupID := obs.StartGroup(sessionID, callID, title, time.Now())
	return withTaskTreeSlot(withTaskTreeObserver(ctx, obs), taskTreeSlot{groupID: groupID}), groupID
}

// finishTreeGroup advances the stamped group row to its terminal state.
func finishTreeGroup(ctx context.Context, callID string, state taskmonitor.TaskState) {
	obs := taskTreeObserverFromContext(ctx)
	if obs == nil {
		return
	}
	slot, ok := taskTreeSlotFromContext(ctx)
	if !ok || slot.groupID == "" {
		return
	}
	obs.FinishGroup(ParentSession(ctx), callID, state)
}

// withParallelChildSlot overrides the base slot with the child's position.
func withParallelChildSlot(ctx context.Context, idx int) context.Context {
	obs := taskTreeObserverFromContext(ctx)
	slot, ok := taskTreeSlotFromContext(ctx)
	if obs == nil || !ok || slot.groupID == "" {
		return ctx
	}
	return withTaskTreeSlot(ctx, taskTreeSlot{groupID: slot.groupID, position: idx + 1})
}

// treeRecording tracks one sub-agent run's row; begin creates it (nil when
// recording is off), finish advances it to its terminal state.
type treeRecording struct {
	observer  taskmonitor.TaskTreeObserver
	sessionID string
	callID    string
}

func beginTreeRecording(ctx context.Context, t *TaskTool, spec ProfileExecSpec) *treeRecording {
	obs := taskTreeObserverFromContext(ctx)
	if obs == nil {
		obs = t.treeObserver
	}
	slot, hasSlot := taskTreeSlotFromContext(ctx)
	if obs == nil || spec.Sched.RunInBackground || taskTreeSuppressed(ctx) {
		return nil
	}
	parentID, position, dependsOn := "", 0, []string(nil)
	if hasSlot {
		parentID, position, dependsOn = slot.groupID, slot.position, slot.dependsOn
	}
	sessionID := ParentSession(ctx)
	callID, _, _, _ := CallContext(ctx)
	obs.StartChild(sessionID, callID, parentID,
		firstNonEmpty(spec.Task.Description, spec.Worker.Name, "task"), position, dependsOn, time.Now())
	return &treeRecording{observer: obs, sessionID: sessionID, callID: callID}
}

func (r *treeRecording) finish(ctxErr, err error) {
	if r == nil {
		return
	}
	r.observer.FinishChild(r.sessionID, r.callID, treeFinishState(ctxErr, err))
}

// treeFinishState maps a sub-agent run's outcome to its terminal task state.
func treeFinishState(ctxErr, err error) taskmonitor.TaskState {
	if ctxErr != nil {
		return taskmonitor.TaskStateCancelled
	}
	if err != nil {
		return taskmonitor.TaskStateFailed
	}
	return taskmonitor.TaskStateSucceeded
}

// treeGroupFinishState maps the group progress phase to its terminal state.
func treeGroupFinishState(phase subagentProgressPhase) taskmonitor.TaskState {
	switch phase {
	case subagentPhaseCancelled:
		return taskmonitor.TaskStateCancelled
	case subagentPhaseFailed:
		return taskmonitor.TaskStateFailed
	default:
		return taskmonitor.TaskStateSucceeded
	}
}

// fleetDependsOnFor resolves the dependency sibling ids for a fleet item.
func fleetDependsOnFor(ctx context.Context, obs taskmonitor.TaskTreeObserver, parentID string, plan fleetPlan, idx int) []string {
	if obs == nil {
		return nil
	}
	sessionID := ParentSession(ctx)
	out := make([]string, 0, len(plan.deps[idx]))
	for _, dep := range plan.deps[idx] {
		out = append(out, obs.TreeTaskID(sessionID, fmt.Sprintf("%s/fleet-%d", parentID, dep+1)))
	}
	return out
}

// recordSkippedFleetChildren persists dependency-cut items as skipped rows.
func recordSkippedFleetChildren(ctx context.Context, obs taskmonitor.TaskTreeObserver, groupID, parentID string, specs []ProfileExecSpec, plan fleetPlan, results []fleetItemResult) {
	if obs == nil || groupID == "" {
		return
	}
	sessionID := ParentSession(ctx)
	for idx, r := range results {
		if r.status != fleetItemSkipped {
			continue
		}
		subID := fmt.Sprintf("%s/fleet-%d", parentID, idx+1)
		title := specs[idx].Task.Description
		if title == "" {
			title = fmt.Sprintf("fleet-%d", idx+1)
		}
		obs.RecordSkipped(sessionID, subID, groupID, title, idx+1, fleetDependsOnFor(ctx, obs, parentID, plan, idx), time.Now())
	}
}

// recordSkippedParallelChildren persists never-dispatched children as skipped.
func recordSkippedParallelChildren(ctx context.Context, parentID string, params []parallelTaskItem, statuses []parallelTaskStatus) {
	obs := taskTreeObserverFromContext(ctx)
	if obs == nil {
		return
	}
	slot, ok := taskTreeSlotFromContext(ctx)
	if !ok || slot.groupID == "" {
		return
	}
	sessionID := ParentSession(ctx)
	for i, st := range statuses {
		if st != parallelTaskSkipped {
			continue
		}
		title := params[i].Description
		if title == "" {
			title = fmt.Sprintf("task-%d", i+1)
		}
		obs.RecordSkipped(sessionID, fmt.Sprintf("%s/sub-%d", parentID, i+1), slot.groupID, title, i+1, nil, time.Now())
	}
}
