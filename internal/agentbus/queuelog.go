package agentbus

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"reasonix/internal/agentbus/jsonl"
	"reasonix/internal/filelock"
)

const (
	queueLogName  = "queue.jsonl"
	queueLockName = "queue.jsonl.lock"
	queueLockWait = 5 * time.Second
)

// QueueLog is the durable parked-work surface of one board. Work is parked when it
// cannot start now and claimed when a slot frees up; the file lives beside the op
// log so another process can pick the work up (AGENT_BUS §13.4).
type QueueLog struct {
	boardDir string
}

// OpenQueueLog addresses the queue stored alongside a board.
func OpenQueueLog(boardDir string) (*QueueLog, error) {
	if boardDir == "" {
		return nil, fmt.Errorf("agentbus: empty board directory")
	}
	abs, err := filepath.Abs(boardDir)
	if err != nil {
		return nil, fmt.Errorf("agentbus: resolve board directory: %w", err)
	}
	if err := jsonl.EnsureDir(abs); err != nil {
		return nil, err
	}
	return &QueueLog{boardDir: abs}, nil
}

// Path is the file the records land in.
func (l *QueueLog) Path() string { return filepath.Join(l.boardDir, queueLogName) }

// Enqueue parks one piece of work and reports whether it was already parked. A
// repeat is not an error: parking the same node twice is the same intent, and the
// node is the queue's identity.
func (l *QueueLog) Enqueue(ctx context.Context, entry QueueEntry, lim QueueLimits) (QueueEntry, bool, error) {
	if entry.Key == "" {
		entry.Key = ParkKey(filepath.Base(l.boardDir), entry.Node, entry.Subtree)
	}
	if entry.EnqueuedAt.IsZero() {
		entry.EnqueuedAt = time.Now().UTC()
	}
	rec, duplicate, err := l.append(ctx, QueueRecord{
		Kind: QueueEnqueue, Node: entry.Node, Subtree: entry.Subtree,
		Participant: entry.Participant, Key: entry.Key, At: entry.EnqueuedAt,
	}, lim)
	if err != nil {
		return QueueEntry{}, false, err
	}
	entry.Seq = rec.Seq
	return entry, duplicate, nil
}

// Claim records that a participant took one piece of parked work out of the queue.
// The lease that makes the work theirs is the board's own claim op: the queue only
// says it is no longer waiting here.
func (l *QueueLog) Claim(ctx context.Context, node, claimant string, lim QueueLimits) (QueueRecord, error) {
	rec, _, err := l.append(ctx, QueueRecord{
		Kind: QueueClaim, Node: node, Participant: claimant, At: time.Now().UTC(),
	}, lim)
	return rec, err
}

// Drop forgets parked work that should never run: a node that was decided against
// is not waiting for anything.
func (l *QueueLog) Drop(ctx context.Context, node string, lim QueueLimits) (QueueRecord, error) {
	rec, _, err := l.append(ctx, QueueRecord{
		Kind: QueueDrop, Node: node, At: time.Now().UTC(),
	}, lim)
	return rec, err
}

// Read folds the whole queue. The counters keep a damaged file honest.
func (l *QueueLog) Read() (*QueueState, jsonl.Read[QueueRecord], error) {
	read, err := jsonl.ReadAll[QueueRecord](l.Path())
	if err != nil {
		return NewQueueState(), read, err
	}
	return FoldQueue(read.Items), read, nil
}

func (l *QueueLog) append(ctx context.Context, rec QueueRecord, lim QueueLimits) (QueueRecord, bool, error) {
	release, err := filelock.AcquireWithExternalTimeout(ctx, filepath.Join(l.boardDir, queueLockName), queueLockWait)
	if err != nil {
		return QueueRecord{}, false, err
	}
	defer release()

	state, read, err := l.Read()
	if err != nil {
		return QueueRecord{}, false, err
	}
	rec.Seq = nextQueueSeq(read.Items)
	duplicate, err := ApplyQueue(state, rec, lim)
	if err != nil {
		return QueueRecord{}, false, err
	}
	if duplicate {
		return rec, true, nil
	}
	if err := jsonl.Append(l.Path(), rec); err != nil {
		return QueueRecord{}, false, err
	}
	return rec, false, nil
}

// nextQueueSeq counts every record, not just the waiting ones: a claim or a drop
// also spends a seq, and forgetting that would mint one twice.
func nextQueueSeq(records []QueueRecord) uint64 {
	var max uint64
	for _, rec := range records {
		if rec.Seq > max {
			max = rec.Seq
		}
	}
	return max + 1
}

// ParkKey derives the identity of one piece of parked work from the work itself,
// never from when it was parked, so re-parking the same node collapses instead of
// piling up.
func ParkKey(board, node, subtree string) string {
	return "agentbus-park:" + board + "/" + subtree + "/" + node
}
