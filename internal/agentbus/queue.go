package agentbus

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// QueueKind is the closed set of queue records. The queue is a log for the same
// reason every other surface is: parked work must survive a restart and be
// claimable from another process (AGENT_BUS §13.4).
type QueueKind string

const (
	QueueEnqueue QueueKind = "enqueue"
	QueueClaim   QueueKind = "claim"
	QueueDrop    QueueKind = "drop"
)

// Queue refusal reasons.
const (
	RefuseQueueNoNode    = "queue_node_missing"
	RefuseQueueDuplicate = "queue_duplicate"
	RefuseQueueDepth     = "queue_full"
	RefuseQueueUnknown   = "queue_unknown_entry"
	RefuseQueueClaimed   = "queue_already_claimed"
	RefuseQueueKind      = "queue_unknown_kind"
)

// QueueLimits bounds the queue. Zero leaves the bound off.
type QueueLimits struct {
	// MaxDepth is how much parked work one board holds before enqueueing is
	// refused: the queue applies backpressure instead of growing without limit.
	MaxDepth int
}

// QueueRecord is one queue record as the log stores it.
type QueueRecord struct {
	Seq         uint64    `json:"seq"`
	Kind        QueueKind `json:"kind"`
	Node        string    `json:"node"`
	Subtree     string    `json:"subtree,omitempty"`
	Participant string    `json:"participant,omitempty"`
	Key         string    `json:"key,omitempty"`
	At          time.Time `json:"at"`
}

// QueueEntry is one piece of parked work.
type QueueEntry struct {
	Node        string
	Subtree     string
	Participant string
	Key         string
	EnqueuedAt  time.Time
	Seq         uint64
}

// QueueState is the folded queue: what is still waiting, in the order it arrived.
type QueueState struct {
	Waiting map[string]QueueEntry
	Dropped map[string]bool
}

// NewQueueState returns an empty queue.
func NewQueueState() *QueueState {
	return &QueueState{Waiting: map[string]QueueEntry{}, Dropped: map[string]bool{}}
}

// FoldQueue replays records in log order.
func FoldQueue(records []QueueRecord) *QueueState {
	st := NewQueueState()
	for _, rec := range records {
		if _, err := ApplyQueue(st, rec, QueueLimits{}); err != nil {
			continue
		}
	}
	return st
}

// QueueReject explains a refused queue record.
type QueueReject struct {
	Node   string
	Kind   QueueKind
	Reason string
}

func (e *QueueReject) Error() string {
	return fmt.Sprintf("agentbus queue: %s refused on %q: %s", e.Kind, e.Node, e.Reason)
}

// IsQueueReject reports whether err is a queue refusal and returns its reason.
func IsQueueReject(err error) (string, bool) {
	var rej *QueueReject
	if !errors.As(err, &rej) {
		return "", false
	}
	return rej.Reason, true
}

// ApplyQueue validates one record and mutates the queue. It reports a duplicate
// enqueue or re-claim instead of failing: replaying the same record must not add
// the same work twice.
func ApplyQueue(st *QueueState, rec QueueRecord, lim QueueLimits) (bool, error) {
	node := strings.TrimSpace(rec.Node)
	if node == "" {
		return false, &QueueReject{Node: rec.Node, Kind: rec.Kind, Reason: RefuseQueueNoNode}
	}
	switch rec.Kind {
	case QueueEnqueue:
		if _, ok := st.Waiting[node]; ok {
			return true, nil
		}
		if _, dropped := st.Dropped[node]; dropped {
			return true, nil
		}
		if lim.MaxDepth > 0 && len(st.Waiting) >= lim.MaxDepth {
			return false, &QueueReject{Node: node, Kind: rec.Kind, Reason: RefuseQueueDepth}
		}
		st.Waiting[node] = QueueEntry{
			Node: node, Subtree: strings.TrimSpace(rec.Subtree), Participant: strings.TrimSpace(rec.Participant),
			Key: rec.Key, EnqueuedAt: rec.At, Seq: rec.Seq,
		}
		return false, nil
	case QueueClaim, QueueDrop:
		if _, ok := st.Waiting[node]; !ok {
			return false, &QueueReject{Node: node, Kind: rec.Kind, Reason: RefuseQueueUnknown}
		}
		delete(st.Waiting, node)
		if rec.Kind == QueueDrop {
			st.Dropped[node] = true
		}
		return false, nil
	default:
		return false, &QueueReject{Node: node, Kind: rec.Kind, Reason: RefuseQueueKind}
	}
}

// Next lists what is waiting, oldest first. Order is arrival order rather than
// priority on purpose: a priority order that never reaches its tail starves it
// silently (AGENT_BUS §13.4).
func (st *QueueState) Next(limit int) []QueueEntry {
	out := make([]QueueEntry, 0, len(st.Waiting))
	for _, entry := range st.Waiting {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].EnqueuedAt.Equal(out[j].EnqueuedAt) {
			return out[i].EnqueuedAt.Before(out[j].EnqueuedAt)
		}
		return out[i].Seq < out[j].Seq
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Depth is how much work is parked.
func (st *QueueState) Depth() int { return len(st.Waiting) }
