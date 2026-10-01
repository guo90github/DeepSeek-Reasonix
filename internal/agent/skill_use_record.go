package agent

// Skill-use record (docs/50 §2.2): which skill a turn ran, and whether that
// skill's bytes have moved since. Fingerprints only — never the skill body.

// SkillUseLimit caps recorded invocations; the oldest fall off.
const SkillUseLimit = 200

// SkillUseRecord is one invocation's fingerprint.
type SkillUseRecord struct {
	TurnSeq int    `json:"turn_seq"`
	Name    string `json:"name"`
	// ContentHash changes whenever the skill's description or body changes.
	ContentHash string `json:"content_hash,omitempty"`
	// CatalogDigest fingerprints the skill catalog the turn's prompt carried.
	CatalogDigest string `json:"catalog_digest,omitempty"`
}

// AppendSkillUse records one invocation, replacing an earlier entry for the same
// turn and skill (a turn may run a skill more than once) and trimming the oldest.
func AppendSkillUse(meta *BranchMeta, use SkillUseRecord) {
	if meta == nil || use.Name == "" {
		return
	}
	kept := meta.SkillUse[:0]
	for _, existing := range meta.SkillUse {
		if existing.TurnSeq != use.TurnSeq || existing.Name != use.Name {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, use)
	if len(kept) > SkillUseLimit {
		kept = kept[len(kept)-SkillUseLimit:]
	}
	meta.SkillUse = kept
}

// LatestSkillUse returns the newest recorded invocation, or false when none.
func LatestSkillUse(meta BranchMeta) (SkillUseRecord, bool) {
	if len(meta.SkillUse) == 0 {
		return SkillUseRecord{}, false
	}
	return meta.SkillUse[len(meta.SkillUse)-1], true
}
