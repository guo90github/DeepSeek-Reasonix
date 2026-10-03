package board

import (
	"fmt"
	"testing"
	"time"
)

// The log is the only truth, so every way of reading a finished log has to agree: replaying the
// lines a file holds and asking the board that wrote them must produce the same state — same
// applied count, same seq, same nodes, same op-id table. The concurrent and torn-log cases pin
// how the log gets written; this pins that its two readers are one reader.
func TestEveryWayOfReadingAFinishedLogAgrees(t *testing.T) {
	b := openTestBoard(t)
	ctx := ctxOf(t)
	for i := range 40 {
		node := fmt.Sprintf("step-%02d", i)
		if _, err := b.Apply(ctx, opAssert(node, "alice")); err != nil {
			t.Fatalf("apply %s: %v", node, err)
		}
	}

	snapshot, err := b.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	lines, err := readLog(b.logPath)
	if err != nil {
		t.Fatalf("readLog: %v", err)
	}
	assertTheSameState(t, "the board that wrote it vs a replay of its lines", snapshot, Fold(lines.Ops))

	reopened, err := Open(b.Dir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	again, err := reopened.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("reopened Snapshot: %v", err)
	}
	assertTheSameState(t, "the board that wrote it vs a fresh open of the same directory", snapshot, again)
}

func assertTheSameState(t *testing.T, what string, want, got *State) {
	t.Helper()
	if want.Applied != got.Applied || want.Rejected != got.Rejected || want.Seq != got.Seq {
		t.Fatalf("%s: applied/rejected/seq = %d/%d/%d vs %d/%d/%d", what,
			want.Applied, want.Rejected, want.Seq, got.Applied, got.Rejected, got.Seq)
	}
	if len(want.Nodes) != len(got.Nodes) || len(want.OpIDs) != len(got.OpIDs) {
		t.Fatalf("%s: %d nodes / %d op ids vs %d / %d", what,
			len(want.Nodes), len(want.OpIDs), len(got.Nodes), len(got.OpIDs))
	}
	for id, node := range want.Nodes {
		other := got.Nodes[id]
		if other == nil || other.State != node.State || other.Owner != node.Owner {
			t.Fatalf("%s: node %s = %+v vs %+v", what, id, node, other)
		}
	}
}
