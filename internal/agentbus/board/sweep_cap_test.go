package board

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestSweepDrainsMoreThanItsCapWithoutStarving is T7-5: one call reclaims at most
// maxSweepPerCall expired claims, and the ones it skips are not lost — a second call
// takes them, in id order. The cap bounds one call's write burst, not the board's
// ability to recover, which is why the order matters and is documented.
func TestSweepDrainsMoreThanItsCapWithoutStarving(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	brd, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	evidence := []Evidence{{Kind: "test", Ref: "sweep-cap"}}
	total := maxSweepPerCall + 20
	for i := range total {
		node := fmt.Sprintf("n%04d", i)
		if _, err := brd.ApplyAll(ctx,
			Op{Verb: VerbAssert, Node: node, Actor: "bench", Evidence: evidence},
			Op{Verb: VerbClaim, Node: node, Actor: "bench",
				Bounds: &Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(60 * time.Millisecond)},
		); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// Every lease lapses at once; nobody heartbeats.
	time.Sleep(120 * time.Millisecond)

	first, err := brd.Sweep(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if len(first) != maxSweepPerCall {
		t.Fatalf("first sweep reclaimed %d, want exactly the cap %d", len(first), maxSweepPerCall)
	}
	second, err := brd.Sweep(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if len(second) != total-maxSweepPerCall {
		t.Fatalf("second sweep reclaimed %d, want the remaining %d", len(second), total-maxSweepPerCall)
	}
	third, err := brd.Sweep(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("third sweep: %v", err)
	}
	if len(third) != 0 {
		t.Fatalf("third sweep reclaimed %d, want nothing left", len(third))
	}

	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for i := range total {
		node := fmt.Sprintf("n%04d", i)
		if owner := state.Nodes[node].Owner; owner != "" {
			t.Fatalf("%s still owned by %q: the cap must not starve anybody", node, owner)
		}
	}
}
