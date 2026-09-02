package taskmonitor

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"reasonix/internal/billing"
	"reasonix/internal/jobs"
)

// TaskRecorder bridges jobs.Manager lifecycle events into the Task Store. It
// is the write side of task monitoring: RecordStart persists a running
// snapshot, RecordDone advances it to its terminal state. All failures are
// swallowed — monitoring is best-effort and must never break the job pipeline.
// The store's per-task lock keeps concurrent recorders (CLI + Desktop) safe.
type TaskRecorder struct {
	store          WriteStore
	projectDir     string
	sessionIDFn    func() string
	mu             sync.Mutex
	monitorIDs     map[string]string
	heartbeats     map[string]context.CancelFunc
	runtimeOwnerID string
}

// NewTaskRecorder returns a TaskRecorder writing to store under projectDir.
// sessionIDFn is called per record so the snapshot reflects the session id at
// creation time (controllers resolve their session path lazily); it may return
// "" when no session is bound yet.
func NewTaskRecorder(store WriteStore, projectDir string, sessionIDFn func() string) *TaskRecorder {
	return &TaskRecorder{store: store, projectDir: projectDir, sessionIDFn: sessionIDFn, monitorIDs: make(map[string]string), heartbeats: make(map[string]context.CancelFunc), runtimeOwnerID: newRuntimeOwnerID()}
}

const (
	runtimeLeaseTTL       = 30 * time.Second
	runtimeHeartbeatEvery = 5 * time.Second
)

var runtimeOwnerSequence atomic.Uint64

func newRuntimeOwnerID() string {
	var nonce [16]byte
	if _, err := crand.Read(nonce[:]); err == nil {
		return hex.EncodeToString(nonce[:])
	}
	h := sha256.Sum256(fmt.Appendf(nil, "%d:%d", timeNow().UnixNano(), runtimeOwnerSequence.Add(1)))
	return hex.EncodeToString(h[:16])
}

// monitorTaskID creates a globally unique monitor identity for a job within
// a session. jobs.Manager IDs are local to a manager and restart from task-1
// for every session, so persisting the raw job ID would cause cross-session
// overwrites in the shared project store.
func monitorTaskID(sessionID, jobID string) string {
	if sessionID == "" {
		return jobID
	}
	id := fmt.Sprintf("%s--%s", sessionID, jobID)
	if len(id) <= maxFieldLen {
		return id
	}
	h := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(h[:8]) + "--" + jobID
}

func sessionlessMonitorTaskID(jobID string) string {
	// Stable per jobID (N4): concurrent recorder instances observing the same
	// sessionless job converge on one monitor row; once a session binds, the
	// session prefix in monitorTaskID disambiguates reused local job ids.
	h := sha256.Sum256([]byte(jobID))
	return hex.EncodeToString(h[:8]) + "--" + jobID
}

func (r *TaskRecorder) rememberMonitorID(jobID, monitorID string) {
	r.mu.Lock()
	r.monitorIDs[jobID] = monitorID
	r.mu.Unlock()
}

func (r *TaskRecorder) lookupMonitorID(jobID string) (string, bool) {
	r.mu.Lock()
	monitorID, ok := r.monitorIDs[jobID]
	r.mu.Unlock()
	return monitorID, ok
}

func (r *TaskRecorder) startHeartbeat(monitorID string) {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if old := r.heartbeats[monitorID]; old != nil {
		old()
	}
	r.heartbeats[monitorID] = cancel
	r.mu.Unlock()
	go func() {
		ticker := time.NewTicker(runtimeHeartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !r.renewHeartbeat(ctx, monitorID) {
					return
				}
			}
		}
	}()
}

func (r *TaskRecorder) renewHeartbeat(ctx context.Context, monitorID string) bool {
	renewed, err := r.store.RenewRuntimeLease(ctx, r.projectDir, monitorID, r.runtimeOwnerID, timeNow().Add(runtimeLeaseTTL))
	return err == nil && renewed
}

func (r *TaskRecorder) stopHeartbeat(monitorID string) {
	r.mu.Lock()
	if cancel := r.heartbeats[monitorID]; cancel != nil {
		cancel()
		delete(r.heartbeats, monitorID)
	}
	r.mu.Unlock()
}

// RecordStart implements jobs.TaskRecorder.
func (r *TaskRecorder) RecordStart(id, kind, label string) {
	ctx := context.Background()
	now := timeNow()
	sessionID := ""
	if r.sessionIDFn != nil {
		sessionID = r.sessionIDFn()
	}
	monitorID := monitorTaskID(sessionID, id)
	if sessionID == "" {
		monitorID = sessionlessMonitorTaskID(id)
	}
	r.rememberMonitorID(id, monitorID)
	snap := TaskSnapshot{
		SchemaVersion:     2,
		TaskID:            monitorID,
		JobID:             id,
		SessionID:         sessionID,
		State:             TaskStateRunning,
		RuntimeState:      RuntimeStateAlive,
		RuntimeLeaseUntil: now.Add(runtimeLeaseTTL),
		RuntimeOwnerID:    r.runtimeOwnerID,
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := r.store.SaveTask(ctx, r.projectDir, snap); err != nil {
		return
	}
	_ = r.store.AppendAuditEvent(ctx, r.projectDir, TaskEvent{
		Timestamp: now, EventType: "state_change",
		TaskID: monitorID, SessionID: sessionID, State: TaskStateRunning,
		RuntimeState: RuntimeStateAlive,
	})
	r.startHeartbeat(monitorID)
}

// RecordDone implements jobs.TaskRecorder.
func (r *TaskRecorder) RecordDone(id string, st jobs.Status, jobErr error) {
	ctx := context.Background()
	target := terminalState(st)
	if target == "" {
		return // non-terminal/unknown status: leave the snapshot untouched
	}
	monitorID, ok := r.lookupMonitorID(id)
	if !ok {
		return // no matching lifecycle was recorded by this recorder
	}
	r.stopHeartbeat(monitorID)
	const maxSaveAttempts = 4
	for range maxSaveAttempts {
		cur, gerr := r.store.GetTask(ctx, r.projectDir, monitorID)
		if gerr != nil || cur == nil {
			return // never recorded (recorder attached after the job started)
		}
		if cur.RuntimeOwnerID != "" && cur.RuntimeOwnerID != r.runtimeOwnerID {
			return // a newer recorder generation owns this reused task identity
		}
		now := timeNow()
		cur.State = target
		cur.RuntimeState = RuntimeStateExited
		cur.RuntimeLeaseUntil = time.Time{}
		cur.RuntimeOwnerID = ""
		cur.Version++
		cur.UpdatedAt = now
		cur.ErrorSummary = ""
		if jobErr != nil {
			cur.ErrorCode = "job_failed"
		}
		if serr := r.store.SaveTask(ctx, r.projectDir, *cur); serr != nil {
			if errors.Is(serr, ErrStoreVersionConflict) {
				continue
			}
			return
		}
		r.appendAuditWithRetry(ctx, TaskEvent{
			Timestamp: now, EventType: "state_change",
			TaskID: monitorID, SessionID: cur.SessionID, State: target,
			RuntimeState: RuntimeStateExited,
			ErrorCode:    cur.ErrorCode, ErrorSummary: cur.ErrorSummary,
		})
		r.prune(ctx)
		return
	}
}

// appendAuditWithRetry writes the audit event, retrying transient store
// failures so a committed SaveTask is not left without its audit trail (B3).
// Best-effort: persistent failure is swallowed like all monitoring.
func (r *TaskRecorder) appendAuditWithRetry(ctx context.Context, ev TaskEvent) {
	const maxAuditAttempts = 3
	for range maxAuditAttempts {
		if err := r.store.AppendAuditEvent(ctx, r.projectDir, ev); err == nil || ctx.Err() != nil {
			return
		}
	}
}

// prune enforces the retention cap after a terminal completion. Best-effort
// like all recording, and cheap for typical task counts: PruneTasks exits
// early when the tree is under the cap.
func (r *TaskRecorder) prune(ctx context.Context) {
	if p, ok := r.store.(interface {
		PruneTasks(context.Context, string, int) (PruneResult, error)
	}); ok {
		_, _ = p.PruneTasks(ctx, r.projectDir, DefaultMaxRetainedTasks)
	}
}

// RecordUsage persists a completed job's cost quote as a cost event and
// updates the snapshot's usage display fields (P1.2/A2). Best-effort; without
// pricing the quote degrades to an unavailable status and the UI hides cost.
func (r *TaskRecorder) RecordUsage(id string, quote *billing.CostQuote, steps int) {
	ctx := context.Background()
	monitorID, ok := r.lookupMonitorID(id)
	if !ok {
		return
	}
	cur, err := r.store.GetTask(ctx, r.projectDir, monitorID)
	if err != nil || cur == nil {
		return
	}
	now := timeNow()
	costTotal, costStatus := formatCostQuote(quote)
	_ = r.store.AppendAuditEvent(ctx, r.projectDir, TaskEvent{
		Timestamp: now, EventType: "cost",
		TaskID: monitorID, SessionID: cur.SessionID,
		State:  cur.State,
		Detail: costEventDetail(quote, steps),
	})
	apply := func(snap *TaskSnapshot) {
		if steps > 0 {
			snap.StepsUsed = steps
		}
		if costTotal != "" {
			snap.CostTotal = costTotal
		}
		snap.CostStatus = costStatus
		snap.Version++
		snap.UpdatedAt = now
	}
	apply(cur)
	for range 4 {
		if err := r.store.SaveTask(ctx, r.projectDir, *cur); err == nil {
			return
		} else if !errors.Is(err, ErrStoreVersionConflict) {
			return
		}
		latest, gerr := r.store.GetTask(ctx, r.projectDir, monitorID)
		if gerr != nil || latest == nil {
			return
		}
		cur = latest
		apply(cur)
	}
}

// formatCostQuote returns a display string and status for a quote; the string
// is empty when cost is unavailable so callers degrade gracefully.
func formatCostQuote(q *billing.CostQuote) (string, string) {
	if q == nil || !q.CostComplete || q.Selected == nil {
		return "", "unavailable"
	}
	return formatMoney(q.Selected), "ok"
}

func formatMoney(m *billing.Money) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m.Currency) + " " + m.Amount
}

func costEventDetail(q *billing.CostQuote, steps int) string {
	cost, _ := formatCostQuote(q)
	if cost == "" {
		cost = "pricing unavailable"
	}
	if steps > 0 {
		return fmt.Sprintf("%s over %d steps", cost, steps)
	}
	return cost
}

// terminalState maps a job status to the task state it reports. Unknown or
// non-terminal statuses map to "" (no update).
func terminalState(st jobs.Status) TaskState {
	switch st {
	case jobs.Done:
		return TaskStateSucceeded
	case jobs.Failed:
		return TaskStateFailed
	case jobs.Killed, jobs.Interrupted:
		return TaskStateCancelled
	default:
		return ""
	}
}
