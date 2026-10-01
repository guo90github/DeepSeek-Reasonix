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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn++
	c.readiness.clear()
	return c.turn
}
