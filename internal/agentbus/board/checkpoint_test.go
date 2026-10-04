package board

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedBoard applies count assertions, so writers take the snapshots a cold read then starts from.
func seedBoard(t *testing.T, count int) string {
	t.Helper()
	dir := t.TempDir()
	brd, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := range count {
		op := assertAs(fmt.Sprintf("hot-%03d", i), time.Now().UTC(), fmt.Sprintf("e%d", i))
		if _, err := brd.Apply(context.Background(), op); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	return dir
}

// A cold handle reads the snapshot and folds only what came after it, and reports the same state a
// full fold of the whole log gives.
func TestAColdHandleStartsFromTheSnapshot(t *testing.T) {
	dir := seedBoard(t, checkpointEvery+44)
	read, err := readLog(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	brd, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if brd.checkpointed != 0 {
		t.Fatalf("a fresh handle claims %d ops from a snapshot, want none before it folds", brd.checkpointed)
	}
	st, err := brd.foldedState()
	if err != nil {
		t.Fatalf("foldedState: %v", err)
	}
	if brd.checkpointed != checkpointEvery {
		t.Fatalf("the snapshot covered %d ops, want the %d a writer took", brd.checkpointed, checkpointEvery)
	}
	whole := Fold(read.Ops)
	for _, probe := range []struct {
		name      string
		fromSnap  int
		fromWhole int
	}{
		{"applied", st.Applied, whole.Applied},
		{"nodes", len(st.Nodes), len(whole.Nodes)},
		{"ids", len(st.OpIDs), len(whole.OpIDs)},
	} {
		if probe.fromSnap != probe.fromWhole {
			t.Fatalf("%s from the snapshot = %d, from the whole log = %d", probe.name, probe.fromSnap, probe.fromWhole)
		}
	}
	if st.Seq != whole.Seq {
		t.Fatalf("seq from the snapshot = %d, from the whole log = %d", st.Seq, whole.Seq)
	}
}

// A snapshot is trusted only when the log still ends where it says it does. Everything else folds
// the whole log, which is slower and always right.
func TestASnapshotThatCannotBeVerifiedIsIgnored(t *testing.T) {
	cases := []struct {
		name  string
		spoil func(t *testing.T, dir string)
	}{
		{"torn file", func(t *testing.T, dir string) {
			path := filepath.Join(dir, checkpointFileName)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read snapshot: %v", err)
			}
			if err := os.WriteFile(path, raw[:len(raw)/2], filePerm); err != nil {
				t.Fatalf("spoil snapshot: %v", err)
			}
		}},
		{"log shrank", func(t *testing.T, dir string) {
			path := filepath.Join(dir, logFileName)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read log: %v", err)
			}
			if err := os.WriteFile(path, raw[:len(raw)/4], filePerm); err != nil {
				t.Fatalf("shrink log: %v", err)
			}
		}},
		{"boundary bytes changed", func(t *testing.T, dir string) {
			// What a snapshot proves is its boundary — a window of the prefix's last bytes, plus the
			// seq of the op that follows — not the whole prefix. Re-reading the prefix to prove it
			// would defeat the snapshot, and the append-only rule is what keeps it intact.
			var ck checkpoint
			raw, err := os.ReadFile(filepath.Join(dir, checkpointFileName))
			if err != nil {
				t.Fatalf("read snapshot: %v", err)
			}
			if err := json.Unmarshal(raw, &ck); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			path := filepath.Join(dir, logFileName)
			logRaw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read log: %v", err)
			}
			logRaw[ck.UptoOffset-8] = 'X'
			if err := os.WriteFile(path, logRaw, filePerm); err != nil {
				t.Fatalf("spoil log: %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := seedBoard(t, checkpointEvery+44)
			if _, err := os.Stat(filepath.Join(dir, checkpointFileName)); err != nil {
				t.Fatalf("the seed should have left a snapshot: %v", err)
			}
			tc.spoil(t, dir)
			brd, err := Open(dir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			st, err := brd.foldedState()
			if err != nil {
				t.Fatalf("foldedState: %v", err)
			}
			if brd.checkpointed != 0 {
				t.Fatalf("the handle used a snapshot it could not verify (%d ops)", brd.checkpointed)
			}
			// Whatever the log now says, the read agrees with a read that skipped no prefix.
			read, err := readLog(filepath.Join(dir, logFileName))
			if err != nil {
				t.Fatalf("read log: %v", err)
			}
			if whole := Fold(read.Ops); st.Applied != whole.Applied {
				t.Fatalf("applied = %d, a whole fold gives %d", st.Applied, whole.Applied)
			}
		})
	}
}

// The snapshot carries the idempotency table, so a retried delivery still collapses after a cold
// start: without it a duplicate would be applied a second time.
func TestADuplicateIdStillCollapsesAfterASnapshot(t *testing.T) {
	dir := seedBoard(t, checkpointEvery+44)
	brd, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := brd.foldedState(); err != nil {
		t.Fatalf("foldedState: %v", err)
	}
	if brd.checkpointed == 0 {
		t.Fatal("this test needs the cold read to have used a snapshot")
	}
	read, err := readLog(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	first := read.Ops[0]
	before := len(read.Ops)
	receipt, err := brd.Apply(context.Background(), first)
	if err != nil {
		t.Fatalf("re-apply the first op: %v", err)
	}
	if !receipt.Duplicate {
		t.Fatal("a retried op id was not recognised as a duplicate after a cold start")
	}
	if receipt.Seq != first.Seq {
		t.Fatalf("the duplicate answered seq %d, want the first write's %d", receipt.Seq, first.Seq)
	}
	after, err := readLog(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("read log again: %v", err)
	}
	if len(after.Ops) != before {
		t.Fatalf("the log grew from %d to %d ops on a retried id", before, len(after.Ops))
	}
}

// Damage the snapshot already folded is still counted, so "what did the read lose" stays honest
// across the boundary instead of resetting at every snapshot.
func TestDamageInThePrefixSurvivesASnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, logFileName)
	brd, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = brd
	for i := range checkpointEvery {
		if err := appendOp(path, assertAs(fmt.Sprintf("hot-%03d", i), time.Now().UTC(), fmt.Sprintf("e%d", i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	plantCorruptLine(t, path, 10)
	writer, err := Open(dir)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	st, err := writer.foldedState()
	if err != nil {
		t.Fatalf("foldedState: %v", err)
	}
	if st.Skipped == 0 {
		t.Fatal("the corrupt line was not counted by the first read")
	}
	// This write is what takes the snapshot: it sees enough applied ops to be worth one.
	if _, err := writer.Apply(context.Background(), assertAs("later", time.Now().UTC(), "e-later")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, checkpointFileName)); err != nil {
		t.Fatalf("the write should have left a snapshot: %v", err)
	}
	cold, err := Open(dir)
	if err != nil {
		t.Fatalf("open cold: %v", err)
	}
	coldState, err := cold.foldedState()
	if err != nil {
		t.Fatalf("cold foldedState: %v", err)
	}
	if cold.checkpointed == 0 {
		t.Fatal("the cold read did not use the snapshot it just gained")
	}
	if coldState.Skipped != st.Skipped {
		t.Fatalf("skipped = %d after the snapshot, %d before it", coldState.Skipped, st.Skipped)
	}
}

// Reads hold the shared lock, so a read may never write a snapshot: only a writer takes one.
func TestAReadNeverWritesASnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, logFileName)
	for i := range checkpointEvery + 8 {
		if err := appendOp(path, assertAs(fmt.Sprintf("hot-%03d", i), time.Now().UTC(), fmt.Sprintf("e%d", i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	brd, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if state.Applied != checkpointEvery+8 {
		t.Fatalf("applied = %d, want the whole log", state.Applied)
	}
	if _, err := os.Stat(filepath.Join(dir, checkpointFileName)); !os.IsNotExist(err) {
		t.Fatalf("a read wrote a snapshot: %v", err)
	}
}

// plantCorruptLine inserts a line that cannot decode, exactly where a damaged record would be.
func plantCorruptLine(t *testing.T, path string, after int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	at := after + 1
	out := append([]string{}, lines[:at]...)
	out = append(out, "{not json")
	out = append(out, lines[at:]...)
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), filePerm); err != nil {
		t.Fatalf("write log: %v", err)
	}
}
