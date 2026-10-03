package board

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkSweepAtLogSize records what a sweep-before-write costs when nothing has
// expired: one lock, one scan, no appended op. Recorded rather than asserted, the
// same way the write and read cost benchmarks are.
func BenchmarkSweepAtLogSize(b *testing.B) {
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
				if _, err := brd.Sweep(ctx, now); err != nil {
					b.Fatalf("sweep: %v", err)
				}
			}
		})
	}
}
