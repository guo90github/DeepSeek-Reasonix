package board

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkSnapshotAtLogSize measures the read path. T4-8's remaining half is about
// this one: a read folds the whole log, so it is O(n) — recorded here so the fix has a
// target, exactly as the write path did (the write half went 6.95 ms → 1.11 ms at
// 1600 ops once it stopped folding everything per write).
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
