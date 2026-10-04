package agentbus

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func ev(ref string) []board.Evidence { return []board.Evidence{{Kind: "test", Ref: ref}} }

func assertOp(node, actor string) board.Op {
	return board.Op{Verb: board.VerbAssert, Node: node, Actor: actor, Evidence: ev("evidence:" + node)}
}

func claimOp(node, actor string) board.Op {
	return board.Op{
		Verb: board.VerbClaim, Node: node, Actor: actor,
		Deadline: time.Now().UTC().Add(time.Hour),
		Bounds:   &board.Bounds{Steps: 1},
	}
}

func doneOp(node, actor, reproducedBy string) board.Op {
	return board.Op{Verb: board.VerbDecide, Node: node, Actor: actor, Outcome: board.OutcomeDone, ReproducedBy: reproducedBy}
}

func requireOp(node, dep string) board.Op {
	return board.Op{Verb: board.VerbRequire, Node: node, Actor: "alice", Dep: &board.NodeSpec{ID: dep}}
}

func testState(t *testing.T, ops ...board.Op) *board.State {
	t.Helper()
	b, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	if _, err := b.ApplyAll(context.Background(), ops...); err != nil {
		t.Fatalf("apply ops: %v", err)
	}
	st, err := b.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return st
}

func nodeIDs(v View) []string {
	out := make([]string, 0, len(v.Lines))
	for _, l := range v.Lines {
		out = append(out, l.ID)
	}
	return out
}

func hasID(v View, id string) bool {
	for _, l := range v.Lines {
		if l.ID == id {
			return true
		}
	}
	return false
}

func TestViewShowsOnlyMyNodesAndMyWaiters(t *testing.T) {
	st := testState(t,
		assertOp("mine", "alice"),
		claimOp("mine", "alice"),
		assertOp("other", "bob"),
		assertOp("waiter", "bob"),
		requireOp("waiter", "mine"),
	)
	v := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"})
	if !hasID(v, "mine") {
		t.Fatalf("my own node missing from my view: %v", nodeIDs(v))
	}
	if !hasID(v, "waiter") {
		t.Fatalf("a node waiting on me must be visible: %v", nodeIDs(v))
	}
	if hasID(v, "other") {
		t.Fatalf("an unrelated participant's node leaked into my view: %v", nodeIDs(v))
	}
	if v.Owned != 1 || v.Waiting != 1 {
		t.Fatalf("counts = owned %d waiting %d, want 1/1", v.Owned, v.Waiting)
	}
}

func TestViewHidesSettledWaiters(t *testing.T) {
	st := testState(t,
		assertOp("mine", "alice"),
		assertOp("settled", "bob"),
		requireOp("settled", "mine"),
		doneOp("settled", "judge", "checker"),
	)
	v := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"})
	if hasID(v, "settled") {
		t.Fatalf("a settled dependent is no longer waiting on me: %v", nodeIDs(v))
	}
}

func TestViewDeliversEveryRowExactlyOnceUnderACap(t *testing.T) {
	st := testState(t,
		assertOp("n1", "alice"),
		assertOp("n2", "alice"),
		assertOp("n3", "alice"),
	)
	first := BuildView(st, ViewSpec{Board: "b1", Participant: "alice", MaxLines: 2})
	if got := nodeIDs(first); len(got) != 2 || got[0] != "n1" || got[1] != "n2" {
		t.Fatalf("first page = %v, want [n1 n2]", got)
	}
	if first.Truncated != 1 {
		t.Fatalf("truncated = %d, want 1 (the cap must be counted)", first.Truncated)
	}
	second := BuildView(st, ViewSpec{Board: "b1", Participant: "alice", MaxLines: 2, Cursor: first.Next})
	if got := nodeIDs(second); len(got) != 1 || got[0] != "n3" {
		t.Fatalf("second page = %v, want [n3]", got)
	}
	if second.Truncated != 0 {
		t.Fatalf("truncated = %d, want 0 once the remainder fits", second.Truncated)
	}
	if first.Next != 2 {
		t.Fatalf("next cursor = %d, want 2 (highest delivered seq)", first.Next)
	}
}

func TestViewCountsWhatItDidNotDeliver(t *testing.T) {
	ops := []board.Op{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		ops = append(ops, assertOp(id, "alice"))
	}
	st := testState(t, ops...)
	v := BuildView(st, ViewSpec{Board: "b1", Participant: "alice", MaxLines: 2})
	if v.Owned != 5 {
		t.Fatalf("owned = %d, want 5: visible-but-undelivered rows still count", v.Owned)
	}
	if v.Truncated != 3 {
		t.Fatalf("truncated = %d, want 3", v.Truncated)
	}
}

func TestViewHeaderIsStableAndBodyIsAppended(t *testing.T) {
	st := testState(t, assertOp("n1", "alice"), assertOp("n2", "alice"))
	base := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"}).Render()
	next := BuildView(st, ViewSpec{Board: "b1", Participant: "alice", Cursor: 1}).Render()
	baseLines := strings.SplitN(base, "\n", 2)
	nextLines := strings.SplitN(next, "\n", 2)
	if baseLines[0] != nextLines[0] {
		t.Fatalf("header changed with the cursor: %q vs %q", baseLines[0], nextLines[0])
	}
	if !strings.HasPrefix(baseLines[0], "agentbus view schema="+SchemaVersion) {
		t.Fatalf("unexpected header %q", baseLines[0])
	}
	if again := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"}).Render(); again != base {
		t.Fatalf("the same state and spec must render identically")
	}
}

func TestViewTruncatesLongTitles(t *testing.T) {
	long := strings.Repeat("观", maxTitleRunes+40)
	st := testState(t,
		assertOp("root", "alice"),
		board.Op{Verb: board.VerbSplit, Node: "root", Actor: "alice", Children: []board.NodeSpec{{ID: "kid", Title: long}}},
	)
	v := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"})
	if !hasID(v, "kid") {
		t.Fatalf("my own dependency must be visible: %v", nodeIDs(v))
	}
	for _, l := range v.Lines {
		if l.ID != "kid" {
			continue
		}
		if got := len([]rune(l.Title)); got != maxTitleRunes {
			t.Fatalf("kid title = %d runes, want %d", got, maxTitleRunes)
		}
	}
}

func TestViewRespectsTheByteCap(t *testing.T) {
	ops := []board.Op{}
	for _, id := range []string{"n1", "n2", "n3", "n4", "n5"} {
		ops = append(ops, assertOp(id, "alice"))
	}
	st := testState(t, ops...)
	// 420, not 400: the row carries the op column now (who wrote the last move, F64), so the
	// same property is asserted against the budget that actually holds a row today.
	const cap = 420
	v := BuildView(st, ViewSpec{Board: "b1", Participant: "alice", MaxBytes: cap})
	if len(v.Lines) == 0 {
		t.Fatalf("at least one row must fit in %d bytes", cap)
	}
	if v.Truncated == 0 {
		t.Fatalf("a %d byte cap cannot hold five rows; truncation must be counted", cap)
	}
	if got := len(v.Render()); got > cap {
		t.Fatalf("render = %d bytes, want <= %d", got, cap)
	}
}

func TestViewOfAnEmptyBoardIsJustTheHeader(t *testing.T) {
	st := testState(t)
	v := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"})
	if len(v.Lines) != 0 || v.Owned != 0 || v.Next != 0 {
		t.Fatalf("empty board view = %+v", v)
	}
	if !strings.Contains(v.Render(), "state cursor=0 next=0") {
		t.Fatalf("counters missing from the empty view: %q", v.Render())
	}
}
