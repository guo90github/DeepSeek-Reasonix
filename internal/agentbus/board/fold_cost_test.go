package board

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// seedLog writes count ops to a fresh board and returns its directory.
func seedLog(b *testing.B, count int) string {
	b.Helper()
	b.StopTimer()
	dir := b.TempDir()
	brd, err := Open(dir)
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	evidence := []Evidence{{Kind: "test", Ref: "bench"}}
	for i := range count {
		node := fmt.Sprintf("n%05d", i)
		if _, err := brd.Apply(ctx, Op{Verb: VerbAssert, Node: node, Actor: "bench", Evidence: evidence}); err != nil {
			b.Fatalf("seed %d: %v", i, err)
		}
	}
	b.StartTimer()
	return dir
}

// BenchmarkApplyAtLogSize measures the cost of one write as the log grows. The
// contract flags the write path as O(n) per write (§11.4 boundary ①); this is the
// number the delta-fold fix has to beat, so it is recorded rather than asserted.
func BenchmarkApplyAtLogSize(b *testing.B) {
	for _, size := range []int{50, 400, 1600} {
		b.Run(fmt.Sprintf("log=%d", size), func(b *testing.B) {
			dir := seedLog(b, size)
			brd, err := Open(dir)
			if err != nil {
				b.Fatalf("open: %v", err)
			}
			ctx := context.Background()
			evidence := []Evidence{{Kind: "test", Ref: "bench"}}
			b.ResetTimer()
			for i := range b.N {
				node := fmt.Sprintf("w%06d", i)
				if _, err := brd.Apply(ctx, Op{
					Verb: VerbAssert, Node: node, Actor: "bench",
					Evidence: evidence, At: time.Now().UTC(),
				}); err != nil {
					b.Fatalf("apply: %v", err)
				}
			}
		})
	}
}
