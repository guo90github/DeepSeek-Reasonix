package board

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func appendRaw(t *testing.T, b *Board, raw string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(b.Dir(), logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(raw); err != nil {
		t.Fatalf("write raw: %v", err)
	}
}

func TestEmptyLogFolds(t *testing.T) {
	b := openTestBoard(t)
	st := snapshot(t, b)
	if st.Seq != 0 || len(st.Nodes) != 0 || st.Applied != 0 {
		t.Fatalf("empty board should fold to an empty state, got %+v", st)
	}
}

func TestTruncatedTailIsNotAnOp(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	appendRaw(t, b, `{"id":"op-torn","verb":"assert","node":"half"`)

	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if len(read.Ops) != 1 {
		t.Fatalf("ops = %d, want 1: a partial line is not an op", len(read.Ops))
	}
	if read.Truncated != 1 || read.Skipped != 0 {
		t.Fatalf("truncated=%d skipped=%d, want 1/0", read.Truncated, read.Skipped)
	}
	st := snapshot(t, b)
	if st.Applied != 1 || st.Truncated != 1 {
		t.Fatalf("applied=%d truncated=%d, want 1/1", st.Applied, st.Truncated)
	}
	if _, ok := st.Nodes["half"]; ok {
		t.Fatalf("a torn line must never become a node")
	}
}

func TestNextWriterRepairsTheTornTail(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	appendRaw(t, b, `{"id":"op-torn","verb":"assert","node":"half"`)

	mustApply(t, b, opAssert("after", "alice"))
	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if len(read.Ops) != 2 || read.Truncated != 0 || read.Skipped != 0 {
		t.Fatalf("ops=%d truncated=%d skipped=%d, want 2/0/0", len(read.Ops), read.Truncated, read.Skipped)
	}
	if read.Ops[1].Node != "after" {
		t.Fatalf("second op = %q, want after", read.Ops[1].Node)
	}
	st := snapshot(t, b)
	if st.Applied != 2 {
		t.Fatalf("applied = %d, want 2", st.Applied)
	}
}

func TestCorruptMidFileLineIsSkippedAndCounted(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	appendRaw(t, b, "not json at all\n")
	mustApply(t, b, opAssert("m", "alice"))

	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if len(read.Ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(read.Ops))
	}
	if read.Skipped != 1 || read.Truncated != 0 {
		t.Fatalf("skipped=%d truncated=%d, want 1/0", read.Skipped, read.Truncated)
	}
	st := snapshot(t, b)
	if st.Applied != 2 || st.Skipped != 1 {
		t.Fatalf("applied=%d skipped=%d, want 2/1", st.Applied, st.Skipped)
	}
}

func TestBlankLineIsCounted(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	appendRaw(t, b, "\n\n")
	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if read.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2", read.Skipped)
	}
}

func TestSeqIsAssignedFromTheLog(t *testing.T) {
	b := openTestBoard(t)
	for i := range 5 {
		rec := mustApply(t, b, opAssert("n-"+itoa(i), "alice"))
		if rec.Seq != uint64(i+1) {
			t.Fatalf("seq = %d, want %d", rec.Seq, i+1)
		}
	}
	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if read.Ops[4].Seq != 5 || len(read.Ops) != 5 {
		t.Fatalf("log tail = %+v", read.Ops[4])
	}
}

func TestLogLineIsExactlyOneJSONObject(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	data, err := os.ReadFile(b.logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatalf("log must end with a newline")
	}
	if got := len(data) - len(trimLastNewline(data)); got != 1 {
		t.Fatalf("expected exactly one trailing newline")
	}
}

func trimLastNewline(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\n' {
		return data[:len(data)-1]
	}
	return data
}

func TestRepairScansPastTheWindow(t *testing.T) {
	b := openTestBoard(t)
	big := make([]byte, 2*repairWindow+16)
	for i := range big {
		big[i] = 'x'
	}
	appendRaw(t, b, string(big))
	repaired, err := repairTornTail(b.logPath)
	if err != nil {
		t.Fatalf("repair must scan past the window instead of failing: %v", err)
	}
	if !repaired {
		t.Fatalf("a log that is one long torn line must be truncated away")
	}
	info, err := os.Stat(b.logPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("size = %d, want 0: nothing complete was in the file", info.Size())
	}
	mustApply(t, b, opAssert("n", "alice"))
	if st := snapshot(t, b); st.Applied != 1 {
		t.Fatalf("applied = %d, want 1", st.Applied)
	}
}

func TestSnapshotReportsReadDamage(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	appendRaw(t, b, `{"id":"x"`)
	before := time.Now().UTC()
	st, err := b.Snapshot(ctxOf(t), before)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if st.Truncated != 1 {
		t.Fatalf("truncated = %d, want 1 (counters must reach the reader)", st.Truncated)
	}
}
