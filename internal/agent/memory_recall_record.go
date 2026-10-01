package agent

// Memory-recall record (docs/50 §2.2): what each turn asked memory for and which
// facts reached the model. It rides the session sidecar for the review page, and
// stays content-free: identifiers, numbers, and the query's hash, never text.

// MemoryRecallTurnLimit caps the recorded turns. The record is a review aid, not
// a ledger; the oldest turns fall off.
const MemoryRecallTurnLimit = 200

// MemoryRecallTurn records one turn's recall decision.
type MemoryRecallTurn struct {
	TurnSeq    int                   `json:"turn_seq"`
	QueryHash  string                `json:"query_hash,omitempty"`
	UsedChars  int                   `json:"used_chars,omitempty"`
	Omitted    int                   `json:"omitted,omitempty"`
	Suppressed string                `json:"suppressed,omitempty"`
	Hits       []MemoryRecallTurnHit `json:"hits,omitempty"`
}

// MemoryRecallTurnHit is one fact's fingerprint in that turn. Injected separates
// the facts the model saw from the ones that matched and were dropped.
type MemoryRecallTurnHit struct {
	ID       string  `json:"id"`
	Revision int     `json:"revision,omitempty"`
	Score    float64 `json:"score,omitempty"`
	Injected bool    `json:"injected,omitempty"`
}

// AppendMemoryRecallTurn records a turn's decision, replacing an earlier entry
// for the same turn (a turn may recall more than once) and trimming the oldest.
func AppendMemoryRecallTurn(meta *BranchMeta, turn MemoryRecallTurn) {
	if meta == nil {
		return
	}
	kept := meta.MemoryRecall[:0]
	for _, existing := range meta.MemoryRecall {
		if existing.TurnSeq != turn.TurnSeq {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, turn)
	if len(kept) > MemoryRecallTurnLimit {
		kept = kept[len(kept)-MemoryRecallTurnLimit:]
	}
	meta.MemoryRecall = kept
}

// LatestMemoryRecallTurn returns the newest record, or false when none exists.
func LatestMemoryRecallTurn(meta BranchMeta) (MemoryRecallTurn, bool) {
	if len(meta.MemoryRecall) == 0 {
		return MemoryRecallTurn{}, false
	}
	return meta.MemoryRecall[len(meta.MemoryRecall)-1], true
}
