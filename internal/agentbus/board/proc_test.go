package board

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const helperEnv = "AGENTBUS_BOARD_HELPER"

// TestMain doubles as the entry point for writer processes the parent kills.
func TestMain(m *testing.M) {
	mode := os.Getenv(helperEnv)
	switch mode {
	case "":
		os.Exit(m.Run())
	case "append", "kill", "torn":
		os.Exit(runWriterHelper(mode))
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode", mode)
		os.Exit(2)
	}
}

// runWriterHelper appends count ops, then either exits cleanly, exits without
// cleanup the way a killed process does, or leaves a partial trailing line.
func runWriterHelper(mode string) int {
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "usage: <mode> <dir> <actor> <count>")
		return 2
	}
	dir, actor := os.Args[2], os.Args[3]
	count, err := strconv.Atoi(os.Args[4])
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad count:", err)
		return 2
	}
	b, err := Open(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open board:", err)
		return 3
	}
	bg := context.Background()
	for i := range count {
		op := Op{
			ID:       fmt.Sprintf("op-%s-%d", actor, i),
			Verb:     VerbAssert,
			Node:     fmt.Sprintf("%s-%d", actor, i),
			Actor:    actor,
			Evidence: []Evidence{{Kind: "test", Ref: fmt.Sprintf("evidence-%s-%d", actor, i)}},
		}
		if _, err := b.Apply(bg, op); err != nil {
			fmt.Fprintln(os.Stderr, "apply:", err)
			return 4
		}
	}
	if mode == "torn" {
		f, err := os.OpenFile(filepath.Join(b.Dir(), logFileName), os.O_APPEND|os.O_WRONLY, filePerm)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open log:", err)
			return 5
		}
		if _, err := f.WriteString(`{"id":"op-torn","verb":"assert","node":"half"`); err != nil {
			fmt.Fprintln(os.Stderr, "write torn line:", err)
			return 5
		}
		_ = f.Close()
	}
	if mode == "kill" || mode == "torn" {
		return 9
	}
	return 0
}

func runWriter(t *testing.T, mode, dir, actor string, count int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	cmd := exec.Command(exe, mode, dir, actor, strconv.Itoa(count))
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	out, err := cmd.CombinedOutput()
	if mode == "append" {
		if err != nil {
			t.Fatalf("helper %s failed: %v\n%s", mode, err, out)
		}
		return
	}
	if err == nil {
		t.Fatalf("helper %s should have exited abruptly", mode)
	}
}

func TestKilledWriterLeavesAFoldableLog(t *testing.T) {
	dir := t.TempDir()
	runWriter(t, "kill", dir, "dead", 5)

	b, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st, err := b.Snapshot(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if st.Applied != 5 {
		t.Fatalf("applied = %d, want 5: fsynced ops survive a killed writer", st.Applied)
	}
	if st.Rejected != 0 || st.Skipped != 0 || st.Truncated != 0 {
		t.Fatalf("log damage: rejected=%d skipped=%d truncated=%d", st.Rejected, st.Skipped, st.Truncated)
	}
	if _, err := b.Apply(ctxOf(t), opAssert("after", "survivor")); err != nil {
		t.Fatalf("the board must stay writable after a writer dies: %v", err)
	}
	st, err = b.Snapshot(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if st.Applied != 6 {
		t.Fatalf("applied = %d, want 6", st.Applied)
	}
}

func TestTornTailFromAKilledWriterIsRepaired(t *testing.T) {
	dir := t.TempDir()
	runWriter(t, "torn", dir, "tornwriter", 2)

	b, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if len(read.Ops) != 2 || read.Truncated != 1 {
		t.Fatalf("ops=%d truncated=%d, want 2/1", len(read.Ops), read.Truncated)
	}
	if _, err := b.Apply(ctxOf(t), opAssert("after", "survivor")); err != nil {
		t.Fatalf("apply after torn tail: %v", err)
	}
	read, err = readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	if len(read.Ops) != 3 || read.Truncated != 0 || read.Skipped != 0 {
		t.Fatalf("ops=%d truncated=%d skipped=%d, want 3/0/0", len(read.Ops), read.Truncated, read.Skipped)
	}
}

func TestConcurrentProcessesDoNotLoseOps(t *testing.T) {
	dir := t.TempDir()
	const writers, perWriter = 3, 6

	done := make(chan struct{})
	for w := range writers {
		go func(w int) {
			defer func() { done <- struct{}{} }()
			runWriter(t, "append", dir, "w"+strconv.Itoa(w), perWriter)
		}(w)
	}
	for range writers {
		<-done
	}

	b, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	read, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	want := writers * perWriter
	if len(read.Ops) != want {
		t.Fatalf("ops = %d, want %d: separate processes lost an op", len(read.Ops), want)
	}
	seen := map[uint64]bool{}
	for _, op := range read.Ops {
		if seen[op.Seq] {
			t.Fatalf("seq %d assigned twice across processes", op.Seq)
		}
		seen[op.Seq] = true
	}
	for seq := uint64(1); seq <= uint64(want); seq++ {
		if !seen[seq] {
			t.Fatalf("seq %d missing", seq)
		}
	}
	st, err := b.Snapshot(ctxOf(t), time.Now().UTC())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if st.Applied != want || st.Rejected != 0 {
		t.Fatalf("applied=%d rejected=%d, want %d/0", st.Applied, st.Rejected, want)
	}
	if len(st.Nodes) != want {
		t.Fatalf("nodes = %d, want %d", len(st.Nodes), want)
	}
}
