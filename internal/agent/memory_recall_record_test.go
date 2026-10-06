package agent

import "testing"

// B2 (docs/50 §2.2): the recall record is bounded, ordered, and one entry per
// turn even when a turn recalls more than once.
func TestAppendMemoryRecallTurnCapsAndReplaces(t *testing.T) {
	meta := &BranchMeta{}
	for turn := 1; turn <= MemoryRecallTurnLimit+5; turn++ {
		AppendMemoryRecallTurn(meta, MemoryRecallTurn{TurnSeq: turn})
	}
	if len(meta.MemoryRecall) != MemoryRecallTurnLimit {
		t.Fatalf("records = %d, want the cap %d", len(meta.MemoryRecall), MemoryRecallTurnLimit)
	}
	if meta.MemoryRecall[0].TurnSeq != 6 {
		t.Fatalf("oldest kept turn = %d, want the oldest five dropped", meta.MemoryRecall[0].TurnSeq)
	}
	if meta.MemoryRecallDropped != 5 {
		t.Fatalf("dropped count = %d, want the five trimmed turns counted", meta.MemoryRecallDropped)
	}

	last := meta.MemoryRecall[len(meta.MemoryRecall)-1].TurnSeq
	AppendMemoryRecallTurn(meta, MemoryRecallTurn{TurnSeq: last, UsedChars: 5, Suppressed: "first"})
	AppendMemoryRecallTurn(meta, MemoryRecallTurn{TurnSeq: last, UsedChars: 9})
	turn, ok := LatestMemoryRecallTurn(*meta)
	if !ok || turn.UsedChars != 9 || turn.Suppressed != "" {
		t.Fatalf("latest turn = %+v, want the replacement to win", turn)
	}
	count := 0
	for _, record := range meta.MemoryRecall {
		if record.TurnSeq == last {
			count++
		}
	}
	if count != 1 || len(meta.MemoryRecall) != MemoryRecallTurnLimit {
		t.Fatalf("turn %d appears %d times over %d records", last, count, len(meta.MemoryRecall))
	}

	if _, ok := LatestMemoryRecallTurn(BranchMeta{}); ok {
		t.Fatal("an empty meta has no latest recall turn")
	}
}
