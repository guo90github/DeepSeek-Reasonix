package board

import (
	"sync"
	"testing"
	"time"
)

func TestFoldIsDeterministic(t *testing.T) {
	ops := []Op{
		opAssert("root", "alice"),
		{ID: "op-1", Seq: 1, Verb: VerbAssert, Node: "root", Actor: "alice", Evidence: evidence("e1")},
	}
	a := Fold(ops)
	b := Fold(ops)
	if a.Applied != b.Applied || a.Seq != b.Seq || len(a.Nodes) != len(b.Nodes) {
		t.Fatalf("fold is not deterministic: %+v vs %+v", a, b)
	}
}

func TestFoldCountsDuplicateIDs(t *testing.T) {
	ops := []Op{
		{ID: "op-dup", Seq: 1, Verb: VerbAssert, Node: "n", Actor: "alice", Evidence: evidence("e")},
		{ID: "op-dup", Seq: 2, Verb: VerbAssert, Node: "n", Actor: "alice", Evidence: evidence("e")},
	}
	st := Fold(ops)
	if st.Applied != 1 {
		t.Fatalf("applied = %d, want 1", st.Applied)
	}
	if st.Rejected != 1 {
		t.Fatalf("rejected = %d, want 1 (duplicate id)", st.Rejected)
	}
	if st.Seq != 2 {
		t.Fatalf("seq = %d, want 2: the log's seq comes from the records, not acceptances", st.Seq)
	}
}

func TestConcurrentApplyFoldsLikeSerial(t *testing.T) {
	b := openTestBoard(t)
	const workers, perWorker = 8, 5

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range perWorker {
				op := opAssert("node-"+itoa(w)+"-"+itoa(i), "worker-"+itoa(w))
				op.ID = "op-" + itoa(w) + "-" + itoa(i)
				if _, err := b.Apply(ctxOf(t), op); err != nil {
					t.Errorf("concurrent apply: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if read.Truncated != 0 || read.Skipped != 0 {
		t.Fatalf("log damage under concurrency: truncated=%d skipped=%d", read.Truncated, read.Skipped)
	}
	if len(read.Ops) != workers*perWorker {
		t.Fatalf("ops = %d, want %d (a lost op means the lock failed)", len(read.Ops), workers*perWorker)
	}
	seen := map[uint64]bool{}
	for _, op := range read.Ops {
		if seen[op.Seq] {
			t.Fatalf("seq %d assigned twice", op.Seq)
		}
		seen[op.Seq] = true
	}
	for seq := uint64(1); seq <= workers*perWorker; seq++ {
		if !seen[seq] {
			t.Fatalf("seq %d missing", seq)
		}
	}
	st := Fold(read.Ops)
	if st.Applied != workers*perWorker || st.Rejected != 0 {
		t.Fatalf("folded applied=%d rejected=%d, want %d/0", st.Applied, st.Rejected, workers*perWorker)
	}
	if len(st.Nodes) != workers*perWorker {
		t.Fatalf("nodes = %d, want %d", len(st.Nodes), workers*perWorker)
	}
}

func TestReadyNeedsEveryDependencyDone(t *testing.T) {
	b := openTestBoard(t)
	bg := ctxOf(t)
	if _, err := b.ApplyAll(bg,
		opAssert("a", "p1"),
		opAssert("b", "p1"),
		opAssert("work", "p2"),
		Op{Verb: VerbRequire, Node: "work", Actor: "p2", Dep: &NodeSpec{ID: "a"}},
		Op{Verb: VerbRequire, Node: "work", Actor: "p2", Dep: &NodeSpec{ID: "b"}},
	); err != nil {
		t.Fatalf("setup: %v", err)
	}
	st := snapshot(t, b)
	if st.Nodes["work"].Ready(st) {
		t.Fatalf("work must not be ready while a is open")
	}
	if _, err := b.Apply(bg, opDecideDone("a", "judge", "checker")); err != nil {
		t.Fatalf("decide a: %v", err)
	}
	st = snapshot(t, b)
	if st.Nodes["work"].Ready(st) {
		t.Fatalf("work must not be ready while b is open")
	}
	if _, err := b.Apply(bg, opDecideDone("b", "judge", "checker")); err != nil {
		t.Fatalf("decide b: %v", err)
	}
	st = snapshot(t, b)
	if !st.Nodes["work"].Ready(st) {
		t.Fatalf("work should be ready once every dependency is done")
	}
	snap := st.Snapshot(time.Now().UTC())
	if len(snap.Ready) != 1 || snap.Ready[0] != "work" {
		t.Fatalf("ready = %v, want [work]", snap.Ready)
	}
}

func TestOpenRejectsEmptyDirectory(t *testing.T) {
	if _, err := Open("  "); err == nil {
		t.Fatalf("Open(\"\") should fail")
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
