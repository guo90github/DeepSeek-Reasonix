package control

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

func (c *Controller) recoverExternallyRemovedSession(path string, saveErr error) (string, error) {
	if c.executor == nil || strings.TrimSpace(path) == "" {
		return "", saveErr
	}
	const reason = "session removed while open"
	req := SessionRecoveryRequest{OriginalPath: path, Reason: reason, Mode: "external-removal"}
	meta := agent.BranchMeta{}
	if c.sessionRecoveryMeta != nil {
		meta = c.sessionRecoveryMeta(req)
	}
	info, err := c.executor.Session().SaveConflictRecoveryBranch(agent.RecoveryBranchOptions{
		OriginalPath: path,
		Reason:       reason,
		BranchMeta:   meta,
	})
	if err != nil {
		return "", fmt.Errorf("preserve externally removed session: %w", err)
	}
	if err := c.commitRecoveredSession(path, reason, info); err != nil {
		return "", err
	}
	appendSnapshotConflictDiagnostic(path, "external-removal", "moved_to_stable_recovery", saveErr, info.Path, info.Existing)
	slog.Warn("controller: active session was removed externally; moved runtime to stable recovery path",
		"path", path, "recovery", info.Path, "existing", info.Existing)
	c.sink.Emit(sessionRecoveryNotice(event.NoticeCodeSessionRecoveryForked,
		"the open session file was removed outside Reasonix; your active conversation was preserved as one recovery copy"))
	return info.Path, nil
}

// snapshotConflictLogAttrs flattens a snapshot-conflict error into slog attrs.
// Field reports of #6069-class "session changed on disk" spam are only
// diagnosable when the logs say which trigger fired and what the revision
// ledger looked like, so every recoverSnapshotConflict outcome logs these.
func snapshotConflictLogAttrs(saveErr error, path, mode string) []any {
	attrs := []any{"path", path, "mode", mode}
	var conflict *agent.SessionSnapshotConflictError
	if errors.As(saveErr, &conflict) && conflict != nil {
		attrs = append(attrs,
			"kind", string(conflict.Kind),
			"disk_messages", conflict.ExistingMessages,
			"snapshot_messages", conflict.SnapshotMessages,
			"base_revision", conflict.BaseRevision,
			"disk_revision", conflict.DiskRevision,
		)
	}
	return attrs
}

// conflictOutcome is recoverSnapshotConflict's declared result. Callers act
// on it directly instead of re-deriving what happened from path or session
// pointer comparisons — the misclassification that broke the depth-cap
// rewrite baseline (#6120) hid in exactly that inference.
type conflictOutcome int

const (
	// conflictDropped: nothing was recovered and the disk transcript could
	// not be adopted; this snapshot was deliberately dropped.
	conflictDropped conflictOutcome = iota
	// conflictAdoptedDisk: the executor session object was replaced by the
	// newer disk transcript; adoptDiskSession already reset its baselines.
	conflictAdoptedDisk
	// conflictForkedBranch: the same in-memory session moved to a freshly
	// forked recovery branch path.
	conflictForkedBranch
)

func sessionRecoveryNotice(code, text string) event.Event {
	return event.Event{
		Kind:     event.Notice,
		Level:    event.LevelWarn,
		Audience: event.NoticeAudienceOperator,
		Code:     code,
		Text:     text,
	}
}
