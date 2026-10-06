package control

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/skill"
)

// RecordSkillUse fingerprints one skill invocation on the session sidecar, so the
// review page can answer "which skill did turn N run, and has it changed since"
// without re-parsing the trajectory. Best-effort: no session path, no write.
func (c *Controller) RecordSkillUse(name, contentHash string) {
	if c == nil {
		return
	}
	name = strings.TrimSpace(name)
	path := strings.TrimSpace(c.SessionPath())
	if name == "" || path == "" {
		return
	}
	use := agent.SkillUseRecord{
		TurnSeq: c.Turn(), Name: name, ContentHash: contentHash,
		CatalogDigest: skillCatalogDigest(skill.CatalogBlock(c.skills.list())),
	}
	_ = agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
		agent.AppendSkillUse(meta, use)
		return nil
	})
}

// skillCatalogDigest fingerprints the catalog text a turn's prompt carried; the
// text itself never enters the record.
func skillCatalogDigest(catalog string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(catalog)))
	return hex.EncodeToString(sum[:8])
}

// nextTurn starts the next turn and returns its number; see Turn.
func (c *Controller) nextTurn() int {
	// Records are keyed on (turn_seq, source): the next number must exceed everything
	// this session's sidecar already holds, or a restart or a session switch would
	// replace a previous run's records instead of adding to them.
	next := lastRecordedTurnSeq(c.SessionPath())
	c.mu.Lock()
	defer c.mu.Unlock()
	if next > c.turn {
		c.turn = next
	}
	c.turn++
	c.readiness.clear()
	return c.turn
}

// lastRecordedTurnSeq is the highest turn number this session's sidecar holds, 0 when
// it holds none yet.
func lastRecordedTurnSeq(path string) int {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		return 0
	}
	max := 0
	for _, turn := range meta.MemoryRecall {
		if turn.TurnSeq > max {
			max = turn.TurnSeq
		}
	}
	return max
}
