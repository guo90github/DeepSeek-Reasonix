package agentbus

import (
	"context"
	"testing"
	"time"
)

func enqueueRec(node string, at time.Time, seq uint64) QueueRecord {
	return QueueRecord{Kind: QueueEnqueue, Node: node, Subtree: "root", At: at, Seq: seq}
}

func reasonOfQueue(_ bool, err error) error { return err }

func TestQueueAppliesEnqueueClaimAndDrop(t *testing.T) {
	st := NewQueueState()
	duplicate, err := ApplyQueue(st, enqueueRec("n1", talkBase, 1), QueueLimits{})
	if err != nil || duplicate {
		t.Fatalf("enqueue = (%v, %v), want a fresh entry", duplicate, err)
	}
	duplicate, err = ApplyQueue(st, enqueueRec("n1", talkBase.Add(time.Minute), 2), QueueLimits{})
	if err != nil || !duplicate {
		t.Fatalf("re-parking the same node = (%v, %v), want a duplicate", duplicate, err)
	}
	if st.Depth() != 1 {
		t.Fatalf("depth = %d, want 1", st.Depth())
	}

	if _, err := ApplyQueue(st, QueueRecord{Kind: QueueClaim, Node: "n1", At: talkBase.Add(time.Minute)}, QueueLimits{}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if st.Depth() != 0 {
		t.Fatalf("depth = %d, want the claimed node gone", st.Depth())
	}
	reason, ok := IsQueueReject(reasonOfQueue(ApplyQueue(st, QueueRecord{Kind: QueueClaim, Node: "n1"}, QueueLimits{})))
	if !ok || reason != RefuseQueueUnknown {
		t.Fatalf("claiming a node nobody parked = (%v, %q), want queue_unknown_entry", ok, reason)
	}

	if _, err := ApplyQueue(st, enqueueRec("n2", talkBase, 3), QueueLimits{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := ApplyQueue(st, QueueRecord{Kind: QueueDrop, Node: "n2", At: talkBase.Add(time.Second)}, QueueLimits{}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	duplicate, err = ApplyQueue(st, enqueueRec("n2", talkBase.Add(2*time.Second), 4), QueueLimits{})
	if err != nil || !duplicate {
		t.Fatalf("a dropped node must not be parked again: (%v, %v)", duplicate, err)
	}
	if st.Depth() != 0 {
		t.Fatalf("depth = %d, want nothing waiting", st.Depth())
	}
}

func TestQueueServesArrivalOrderNotPriority(t *testing.T) {
	st := NewQueueState()
	// Fed newest-first on purpose: the queue must still serve by arrival, because a
	// priority order that never reaches its tail starves it silently.
	for _, rec := range []QueueRecord{
		enqueueRec("late", talkBase.Add(2*time.Minute), 3),
		enqueueRec("early", talkBase, 1),
		enqueueRec("middle", talkBase.Add(time.Minute), 2),
	} {
		if _, err := ApplyQueue(st, rec, QueueLimits{}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	next := st.Next(0)
	want := []string{"early", "middle", "late"}
	if len(next) != len(want) {
		t.Fatalf("next = %+v, want %v", next, want)
	}
	for i, node := range want {
		if next[i].Node != node {
			t.Fatalf("order = %+v, want %v", next, want)
		}
	}
	if head := st.Next(2); len(head) != 2 || head[1].Node != "middle" {
		t.Fatalf("limited head = %+v, want the first two", head)
	}
}

func TestQueueAppliesBackpressureAtItsDepth(t *testing.T) {
	st := NewQueueState()
	lim := QueueLimits{MaxDepth: 1}
	if _, err := ApplyQueue(st, enqueueRec("n1", talkBase, 1), lim); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	reason, ok := IsQueueReject(reasonOfQueue(ApplyQueue(st, enqueueRec("n2", talkBase.Add(time.Second), 2), lim)))
	if !ok || reason != RefuseQueueDepth {
		t.Fatalf("past the depth = (%v, %q), want queue_full", ok, reason)
	}
	if st.Depth() != 1 {
		t.Fatalf("depth = %d: a refused enqueue must not change the queue", st.Depth())
	}
}

func TestQueueLogMintsDistinctSeqAcrossClaims(t *testing.T) {
	log, err := OpenQueueLog(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	first, duplicate, err := log.Enqueue(ctx, QueueEntry{Node: "n1"}, QueueLimits{})
	if err != nil || duplicate {
		t.Fatalf("enqueue = (%v, %v)", duplicate, err)
	}
	if _, err := log.Claim(ctx, "n1", "alice", QueueLimits{}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	second, _, err := log.Enqueue(ctx, QueueEntry{Node: "n2"}, QueueLimits{})
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if second.Seq <= first.Seq {
		t.Fatalf("seq = %d then %d: a claim spends a seq too", first.Seq, second.Seq)
	}
	state, read, err := log.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Items) != 3 || read.Skipped != 0 || read.Truncated != 0 {
		t.Fatalf("read = %+v, want three clean records", read)
	}
	if state.Depth() != 1 {
		t.Fatalf("depth = %d, want only the node nobody claimed", state.Depth())
	}
}
