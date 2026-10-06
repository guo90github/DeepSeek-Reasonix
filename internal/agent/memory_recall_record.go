package agent

// Memory-recall record (docs/50 §2.2): what each turn asked memory for and which facts reached
// the model. It rides the session sidecar for the review page and stays content-free apart from
// each hit's short label — a fact's body never appears.

// MemoryRecallTurnLimit caps the recorded turns. The record is a review aid, not
// a ledger; the oldest turns fall off.
const MemoryRecallTurnLimit = 200

// MemoryRecallTurn records one turn's recall decision.
type MemoryRecallTurn struct {
	TurnSeq   int    `json:"turn_seq"`
	QueryHash string `json:"query_hash,omitempty"`
	// SnapshotDigest fingerprints the session-context the turn actually saw.
	SnapshotDigest string                `json:"snapshot_digest,omitempty"`
	UsedChars      int                   `json:"used_chars,omitempty"`
	Omitted        int                   `json:"omitted,omitempty"`
	Suppressed     string                `json:"suppressed,omitempty"`
	Hits           []MemoryRecallTurnHit `json:"hits,omitempty"`
}

// MemoryRecallTurnHit is one fact's fingerprint in that turn. Injected separates
// the facts the model saw from the ones that matched and were dropped; nil means
// the record predates the field, which is neither. Name and Title are recorded at
// write time so a reader can tell the ids apart without a live controller.
type MemoryRecallTurnHit struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
	// Description is the fact's one-line hook, never its body (docs/50 §2.2): the
	// review page cross-checks a hit against what it claimed to be about.
	Description string `json:"description,omitempty"`
	// Reason names the terms that matched (at most four), so a polluted turn can be
	// explained after the fact instead of guessed at.
	Reason string `json:"reason,omitempty"`
	// Scope, Type and Freshness are the fact's own state as of that turn, so the
	// review page can explain why a fact did or did not apply.
	Scope     string  `json:"scope,omitempty"`
	Type      string  `json:"type,omitempty"`
	Freshness string  `json:"freshness,omitempty"`
	Revision  int     `json:"revision,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Injected  *bool   `json:"injected,omitempty"`
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
	if dropped := len(kept) - MemoryRecallTurnLimit; dropped > 0 {
		kept = kept[dropped:]
		meta.MemoryRecallDropped += dropped
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
