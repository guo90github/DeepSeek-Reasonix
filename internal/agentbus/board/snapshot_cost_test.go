package board

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkSnapshotAtLogSize measures a warm handle's read: the state is already folded, so what
// it costs is a stat and a clone. The cold half T4-8 named ("a read folds the whole log, so it is
// O(n)") is BenchmarkColdSnapshotAtLogSize below, which opens a new handle every iteration.
func BenchmarkSnapshotAtLogSize(b *testing.B) {
	for _, size := range []int{50, 400, 1600} {
		b.Run(fmt.Sprintf("log=%d", size), func(b *testing.B) {
			dir := seedLog(b, size)
			brd, err := Open(dir)
			if err != nil {
				b.Fatalf("open: %v", err)
			}
			ctx := context.Background()
			now := time.Now().UTC()
			b.ResetTimer()
			for range b.N {
				if _, err := brd.Snapshot(ctx, now); err != nil {
					b.Fatalf("snapshot: %v", err)
				}
			}
		})
	}
}

// BenchmarkColdSnapshotAtLogSize measures the read a fresh process pays, in the two shapes that
// decide what a snapshot is worth: one node per op (the folded state ends up as large as the log,
// so skipping the prefix saves little) and sixteen ops per node (the state stays small while the
// log grows). "whole log" drops the snapshot first, so each pair is the before and after of one
// workload rather than two runs at different times. Reads never write a snapshot, so the dropped
// file stays dropped for the rest of that sub-benchmark.
func BenchmarkColdSnapshotAtLogSize(b *testing.B) {
	for _, shape := range []struct {
		name    string
		perNode int
	}{{"node-per-op", 1}, {"ops-per-node", 16}} {
		for _, size := range []int{400, 1600, 6400} {
			for _, from := range []string{"snapshot", "whole log"} {
				b.Run(fmt.Sprintf("%s/log=%d/%s", shape.name, size, from), func(b *testing.B) {
					dir := seedLogShape(b, size, shape.perNode)
					ctx := context.Background()
					now := time.Now().UTC()
					b.ResetTimer()
					for range b.N {
						if from == "whole log" {
							if err := os.Remove(filepath.Join(dir, checkpointFileName)); err != nil && !os.IsNotExist(err) {
								b.Fatalf("drop snapshot: %v", err)
							}
						}
						brd, err := Open(dir)
						if err != nil {
							b.Fatalf("open: %v", err)
						}
						if _, err := brd.Snapshot(ctx, now); err != nil {
							b.Fatalf("snapshot: %v", err)
						}
					}
				})
			}
		}
	}
}

// seedLogShape writes count ops spread over count/perNode nodes. Distinct evidence keeps the op
// ids distinct, so every op is a real op rather than a retry of the previous one.
func seedLogShape(b *testing.B, count, perNode int) string {
	b.Helper()
	b.StopTimer()
	dir := b.TempDir()
	brd, err := Open(dir)
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	for i := range count {
		op := Op{
			Verb: VerbAssert, Node: fmt.Sprintf("n%05d", i/perNode), Actor: "bench",
			Evidence: []Evidence{{Kind: "test", Ref: fmt.Sprintf("bench-%d", i)}},
		}
		if _, err := brd.Apply(ctx, op); err != nil {
			b.Fatalf("seed %d: %v", i, err)
		}
	}
	b.StartTimer()
	return dir
}
