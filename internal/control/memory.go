package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/memory"
)

// memoryManager owns the session's loaded memory snapshot, the queue of pending
// standing-document notes, and the serialization of memory writes — behind its own locks
// and off the controller's c.mu. Like goalMachine it is a strict leaf: its
// methods only touch its own state and never call back into the Controller, so a
// memory-panel save can't stall an approval or status poll on c.mu.
//
// set is an immutable snapshot: reads take mu briefly and return the pointer.
// Writes are serialized by writeMu and do their disk I/O (the doc/store write
// plus the memory.Load re-discovery) OFF mu, taking mu only to swap the freshly
// discovered snapshot in and queue the turn-tail note — so a write never holds a
// lock across a filesystem walk. Standing-document edits queue a compatibility
// note because their authoritative copy remains in system until reload. Background
// fact writes only refresh set; the next real user turn publishes the replacement
// session-context snapshot. All write methods are no-ops returning "" when memory
// is disabled (set == nil).
type memoryManager struct {
	// mu guards set (the snapshot pointer) and pending (the turn-tail queue);
	// every critical section under it is short and non-blocking.
	mu  sync.Mutex
	set *memory.Set
	// pending holds standing-document notes added mid-session (via "#" quick-add
	// or a doc edit). Compose drains them onto the next outgoing turn. Background
	// facts never enter this queue; their live replacement snapshot is injected by
	// the turn-context path.
	pending    []string
	lastRecall memory.RecallResult
	autoWrites map[[32]byte]int

	// writeMu serializes memory writes so each write+reload+swap is atomic with
	// respect to the others. Taken OFF mu, so a read (current/drainPending) never
	// blocks behind a write's disk I/O.
	writeMu sync.Mutex
}

func (m *memoryManager) authorizeAutoRemember(args json.RawMessage) {
	key := sha256.Sum256(args)
	m.mu.Lock()
	if m.autoWrites == nil {
		m.autoWrites = map[[32]byte]int{}
	}
	m.autoWrites[key]++
	m.mu.Unlock()
}

func (m *memoryManager) revokeAutoRemember(args json.RawMessage) {
	key := sha256.Sum256(args)
	m.mu.Lock()
	delete(m.autoWrites, key)
	m.mu.Unlock()
}

func (m *memoryManager) clearAutoRemember() {
	m.mu.Lock()
	m.autoWrites = nil
	m.mu.Unlock()
}

func (m *memoryManager) claimAutoRemember(args json.RawMessage) bool {
	key := sha256.Sum256(args)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.autoWrites[key] <= 0 {
		return false
	}
	if m.autoWrites[key] == 1 {
		delete(m.autoWrites, key)
	} else {
		m.autoWrites[key]--
	}
	return true
}

func (m *memoryManager) recall(query string) memory.RecallResult {
	result := m.current().AutoRecall(query, memory.RecallOptions{})
	m.recordRecall(result)
	return result
}

func (m *memoryManager) recordRecall(result memory.RecallResult) {
	m.mu.Lock()
	m.lastRecall = result
	m.mu.Unlock()
}

func (m *memoryManager) lastRecallResult() memory.RecallResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRecall
}

func newMemoryManager(set *memory.Set) memoryManager {
	return memoryManager{set: set}
}

// memoryRecallAudit strips a recall decision to its content-free fingerprint
// for the trajectory/telemetry channel.
func memoryRecallAudit(result memory.RecallResult) event.MemoryRecallAudit {
	audit := event.MemoryRecallAudit{
		UsedChars: result.UsedChars, Omitted: result.Omitted, Suppressed: result.Suppressed,
		TurnSeq: result.TurnSeq,
	}
	for _, hit := range result.Hits {
		audit.Hits = append(audit.Hits, memoryRecallHit(hit, true))
	}
	// Dropped hits are recorded as fingerprints only: they never reached the
	// model, and the record must show why a fact a user expected was absent.
	for _, hit := range result.Dropped {
		audit.Hits = append(audit.Hits, memoryRecallHit(hit, false))
	}
	for _, hit := range result.ShadowHits {
		audit.Shadow = append(audit.Shadow, event.MemoryRecallHit{ID: hit.ID, Score: hit.Score})
	}
	return audit
}

// current returns the loaded snapshot (nil when memory is disabled). The returned
// *Set is immutable — mutations go through quickAdd / saveDoc / saveMemory.
func memoryRecallHit(hit memory.RecallHit, injected bool) event.MemoryRecallHit {
	return event.MemoryRecallHit{
		ID: hit.Memory.ID, Revision: hit.Memory.Revision, Injected: injected,
		Scope:     string(memory.NormalizeFactScope(string(hit.Memory.Scope))),
		Type:      string(memory.NormalizeType(string(hit.Memory.Type))),
		Freshness: hit.Freshness, Score: hit.Score,
	}
}

func (m *memoryManager) current() *memory.Set {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.set
}

// drainPending returns and clears the queued turn-tail notes, for Compose to fold
// onto the next outgoing turn.
func (m *memoryManager) drainPending() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	notes := m.pending
	m.pending = nil
	return notes
}

// applyWrite re-discovers memory from disk (off-lock, the expensive part) then,
// under a brief mu, swaps the fresh snapshot in and queues the turn-tail note so a
// later current() reflects the just-applied write. mem is the snapshot taken at
// the start of the writeMu-serialized write and supplies the discovery roots.
// Callers hold writeMu.
func (m *memoryManager) applyWrite(mem *memory.Set, note string) {
	reloaded := memory.Load(memory.Options{CWD: mem.CWD, UserDir: mem.UserDir})
	m.mu.Lock()
	if note != "" {
		m.pending = append(m.pending, note)
	}
	m.set = reloaded
	m.mu.Unlock()
}

// applyBackgroundWrite refreshes the live background-memory snapshot without
// generating a legacy <memory-update>. The next real user turn observes the new
// BackgroundDataBlock and appends one complete replacement session-context.
func (m *memoryManager) applyBackgroundWrite(mem *memory.Set) {
	m.applyWrite(mem, "")
}

// quickAdd appends a one-line note to the doc-memory file for scope (project
// REASONIX.md by default) — the write side of "#<note>". Returns the file written.
func (m *memoryManager) quickAdd(scope memory.Scope, note string) (string, error) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return "", nil
	}
	path := mem.DocPath(scope)
	if path == "" {
		return "", fmt.Errorf("no target file for memory scope %q", scope)
	}
	if err := memory.AppendDoc(path, note); err != nil {
		return "", err
	}
	m.applyWrite(mem, note)
	return path, nil
}

// saveDoc overwrites a recognized memory doc with body — the save side of the
// desktop panel's in-place editor. Returns the file written.
func (m *memoryManager) saveDoc(path, body string) (string, error) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return "", nil
	}
	written, err := mem.WriteDoc(path, body)
	if err != nil {
		return "", err
	}
	// Inject the new content once on the next turn: the cached prefix still holds
	// the pre-edit version this session, so handing the model the current text
	// avoids a stale-guidance gap until the next session re-folds it into the
	// prefix. Trimmed to a single tail note (drained by Compose), not per-turn.
	m.applyWrite(mem,
		"Memory file "+written+" was just edited. Its current contents:\n"+strings.TrimSpace(body))
	return written, nil
}

// saveMemory writes an active auto-memory fact and refreshes the in-session
// snapshot. It is the explicit user-confirmed counterpart to the model-owned
// remember tool, used by management surfaces that preview a candidate first.
func (m *memoryManager) saveMemory(fact memory.Memory) (string, error) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return "", nil
	}
	saved, err := mem.Store.SaveWithOptions(fact, memory.SaveOptions{AllowOverCap: true})
	if err != nil {
		return "", err
	}
	m.applyBackgroundWrite(mem)
	return saved.Path, nil
}

// forget removes a saved auto-memory by name — the panel/TUI forget action, the
// manual counterpart to the model's `forget` tool. The file is archived for
// traceability by Store.Delete; the next real turn publishes the new snapshot.
func (m *memoryManager) forget(name string) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return nil
	}
	if err := mem.Store.Delete(name); err != nil {
		return err
	}
	m.applyBackgroundWrite(mem)
	return nil
}

func (m *memoryManager) revisions(ref string) []memory.Memory {
	mem := m.current()
	if mem == nil {
		return nil
	}
	return mem.Store.Revisions(ref)
}

func (m *memoryManager) restore(ref string, revision int) (memory.Memory, error) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return memory.Memory{}, fmt.Errorf("memory unavailable")
	}
	result, err := mem.Store.Restore(ref, revision)
	if err != nil {
		return memory.Memory{}, err
	}
	m.applyBackgroundWrite(mem)
	return result.Memory, nil
}

func (m *memoryManager) restoreArchived(archivePath string) (memory.Memory, error) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return memory.Memory{}, fmt.Errorf("memory unavailable")
	}
	result, err := mem.Store.RestoreArchived(archivePath)
	if err != nil {
		return memory.Memory{}, err
	}
	m.applyBackgroundWrite(mem)
	return result.Memory, nil
}

// queue is the model remember/forget tool callback. The tool result already
// reports the mutation inside the current loop; only the refreshed background
// snapshot is needed for the next real user turn.
func (m *memoryManager) queue(_ string) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if mem := m.current(); mem != nil {
		m.applyBackgroundWrite(mem)
	}
}

// rememberWriteNote appends the write assessment's explanation to an approval
// reason. A remember call that cannot be auto-written usually hit a duplicate or
// a global-scope default, and the reason names the existing fact to update — the
// human sees it in the prompt and the model sees it if the call is denied.
func (c *Controller) rememberWriteNote(tool string, args json.RawMessage, reason string) string {
	if tool != memoryRememberTool {
		return reason
	}
	mem := c.Memory()
	if mem == nil {
		return reason
	}
	assessment := memory.AssessRememberWrite(mem.Store, args)
	if assessment.AutoAllow || strings.TrimSpace(assessment.Reason) == "" {
		return reason
	}
	return combineApprovalReasons(reason, assessment.Reason)
}

// recordMemoryRecallTurn persists one turn's decision on the session sidecar, so
// the review page can answer "which facts did turn N use" without re-parsing the
// trajectory. Best-effort: a missing path or an unwritable sidecar never fails
// the turn.
func (c *Controller) recordMemoryRecallTurn(result memory.RecallResult) {
	path := strings.TrimSpace(c.SessionPath())
	if path == "" {
		return
	}
	turn := agent.MemoryRecallTurn{
		TurnSeq: result.TurnSeq, QueryHash: recallQueryHash(result.Query),
		QueryExcerpt:   recallQueryExcerpt(result.Query),
		SnapshotDigest: c.executorTurnDigest(),
		UsedChars:      result.UsedChars, Omitted: result.Omitted, Suppressed: result.Suppressed,
	}
	for _, hit := range result.Hits {
		injected := true
		turn.Hits = append(turn.Hits, agent.MemoryRecallTurnHit{
			ID: hit.Memory.ID, Name: hit.Memory.Name, Title: hit.Memory.Title,
			Description: hit.Memory.Description, Reason: hit.Reason,
			Scope: string(hit.Memory.Scope), Type: string(hit.Memory.Type), Freshness: hit.Freshness,
			Revision: hit.Memory.Revision, Score: hit.Score, Injected: &injected,
		})
	}
	for _, hit := range result.Dropped {
		injected := false
		turn.Hits = append(turn.Hits, agent.MemoryRecallTurnHit{
			ID: hit.Memory.ID, Name: hit.Memory.Name, Title: hit.Memory.Title,
			Description: hit.Memory.Description, Reason: hit.Reason,
			Scope: string(hit.Memory.Scope), Type: string(hit.Memory.Type), Freshness: hit.Freshness,
			Revision: hit.Memory.Revision, Score: hit.Score, Injected: &injected,
		})
	}
	if len(turn.Hits) == 0 && turn.Suppressed == "" {
		return
	}
	_ = agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
		agent.AppendMemoryRecallTurn(meta, turn)
		return nil
	})
}

// recallQueryExcerptRunes bounds the local excerpt so a long turn cannot bloat the
// sidecar; the transcript beside it still holds the whole text.
const recallQueryExcerptRunes = 60

func recallQueryExcerpt(query string) string {
	line := strings.Join(strings.Fields(query), " ")
	if runes := []rune(line); len(runes) > recallQueryExcerptRunes {
		return string(runes[:recallQueryExcerptRunes]) + "…"
	}
	return line
}

// recallQueryHash keeps the record content-free: the query's text stays out.
func recallQueryHash(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(query))
	return hex.EncodeToString(sum[:8])
}
