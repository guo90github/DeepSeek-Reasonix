package taskmonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DefaultMaxRetainedTasks is the number of most-recent terminal tasks kept per
// project. Older terminal tasks are archived by PruneTasks. Configurable per
// call; a non-positive value selects this default.
const DefaultMaxRetainedTasks = 500

// PruneResult reports what PruneTasks did.
type PruneResult struct {
	Archived    int    `json:"archived"`
	ArchivedDir string `json:"archivedDir,omitempty"`
	Total       int    `json:"total"`
}

// PruneTasks archives terminal tasks beyond maxRetained, oldest first, moving
// each task directory to <taskRoot>/../tasks-archive — a sibling still under
// .reasonix, so the P0.1 git exclusion keeps archived data out of the repo.
// Non-terminal tasks are never touched. Moving (not deleting) keeps retention
// reversible; a later deletion pass can age the archive itself. When an
// archived id already exists (a requeued task finished again), the older copy
// is replaced: the active directory carries the full accumulated event log.
func (s *FileStore) PruneTasks(ctx context.Context, projectDir string, maxRetained int) (PruneResult, error) {
	var out PruneResult
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if maxRetained <= 0 {
		maxRetained = DefaultMaxRetainedTasks
	}
	root, err := s.taskRoot(projectDir)
	if err != nil {
		return out, err
	}
	if err := rejectSymlink(root); err != nil {
		return out, err
	}
	tasks, err := s.ListTasks(ctx, projectDir)
	if err != nil {
		return out, err
	}
	terminal := make([]TaskSnapshot, 0, len(tasks))
	for _, snap := range tasks {
		if snap.State.Terminal() {
			terminal = append(terminal, snap)
		}
	}
	out.Total = len(terminal)
	if len(terminal) <= maxRetained {
		return out, nil
	}
	// ListTasks sorts newest-first, so the tail is the oldest terminal set.
	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].UpdatedAt.Before(terminal[j].UpdatedAt)
	})
	archiveDir := filepath.Join(filepath.Dir(root), "tasks-archive")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return out, fmt.Errorf("prune: create archive: %w", err)
	}
	_ = os.Chmod(archiveDir, 0o700)
	for _, snap := range terminal[:len(terminal)-maxRetained] {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		// Re-read under the same terminal decision: a requeue between listing
		// and archiving must keep the task in the active tree.
		cur, gerr := s.GetTask(ctx, projectDir, snap.TaskID)
		if gerr != nil || cur == nil || !cur.State.Terminal() {
			continue
		}
		src := filepath.Join(root, snap.TaskID)
		dst := filepath.Join(archiveDir, snap.TaskID)
		if err := rejectSymlinkChain(root, src); err != nil {
			continue
		}
		if err := rejectSymlinkChain(archiveDir, dst); err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			// Replaced by a newer terminal run of the same id (requeue).
			if err := os.RemoveAll(dst); err != nil {
				continue
			}
		}
		if err := os.Rename(src, dst); err != nil {
			continue
		}
		out.Archived++
		if s.sink != nil {
			s.sink.SnapshotChanged(projectDir, snap.TaskID)
		}
	}
	out.ArchivedDir = archiveDir
	return out, nil
}
