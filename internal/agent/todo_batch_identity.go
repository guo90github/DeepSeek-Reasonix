package agent

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"reasonix/internal/evidence"
)

// TodoBatchIdentity answers "which batch are these todos?" without hashing the
// list. Editing a plan keeps its batch, so a batch the user closed stays closed
// when the model adds a step or rewords an item; a fresh id is issued only for
// work that shares nothing with the closed batch.
type TodoBatchIdentity struct {
	ID       string   `json:"id,omitempty"`
	Contents []string `json:"contents,omitempty"`
	// Closed records the user's close for this batch. It survives edits of the
	// same work; that is the whole point of a stable identity.
	Closed bool `json:"closed,omitempty"`
}

// todoBatchContentLimit bounds what one identity record may carry: the item
// texts are compared for overlap, never replayed.
const todoBatchContentLimit = 200

// TodoBatchContents reduces a list to the texts an identity compares.
func TodoBatchContents(items []evidence.TodoItem) []string {
	out := make([]string, 0, min(len(items), todoBatchContentLimit))
	for _, item := range items {
		text := strings.TrimSpace(item.Content)
		if text == "" {
			continue
		}
		out = append(out, text)
		if len(out) == todoBatchContentLimit {
			break
		}
	}
	return out
}

// ResolveTodoBatchIdentity returns the batch this list belongs to. newID is the
// generator for a fresh batch (injected so the transitions are testable).
//
// The transitions are deliberate:
//   - no previous batch            -> issue one;
//   - the batch is still open      -> keep it and adopt the edit;
//   - the batch was closed and the
//     list still overlaps it       -> keep it, still closed;
//   - the batch was closed and the
//     list shares nothing with it  -> genuinely new work, issue a fresh id.
func ResolveTodoBatchIdentity(prev TodoBatchIdentity, contents []string, newID func() string) TodoBatchIdentity {
	if len(contents) == 0 {
		return prev
	}
	if strings.TrimSpace(prev.ID) == "" {
		return TodoBatchIdentity{ID: nextTodoBatchID(newID), Contents: contents}
	}
	if !prev.Closed || sharesTodoBatchContent(prev.Contents, contents) {
		return TodoBatchIdentity{ID: prev.ID, Contents: contents, Closed: prev.Closed}
	}
	return TodoBatchIdentity{ID: nextTodoBatchID(newID), Contents: contents}
}

func nextTodoBatchID(newID func() string) string {
	if newID != nil {
		if id := strings.TrimSpace(newID()); id != "" {
			return id
		}
	}
	return NewTodoBatchID()
}

// NewTodoBatchID issues a batch id. It carries no meaning beyond uniqueness.
func NewTodoBatchID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "todos-unknown"
	}
	return "todos-" + hex.EncodeToString(buf[:])
}

func sharesTodoBatchContent(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, text := range left {
		seen[text] = struct{}{}
	}
	for _, text := range right {
		if _, ok := seen[text]; ok {
			return true
		}
	}
	return false
}

// LoadTodoBatchIdentity reads the batch record from the session sidecar.
func LoadTodoBatchIdentity(sessionPath string) TodoBatchIdentity {
	meta, ok, err := LoadBranchMeta(sessionPath)
	if err != nil || !ok || meta.TodoBatch == nil {
		return TodoBatchIdentity{}
	}
	identity := *meta.TodoBatch
	identity.Contents = append([]string(nil), identity.Contents...)
	return identity
}

// SaveTodoBatchIdentity records the batch on the session sidecar, next to the
// dismissals it belongs with.
func SaveTodoBatchIdentity(sessionPath string, identity TodoBatchIdentity) error {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" || strings.TrimSpace(identity.ID) == "" {
		return nil
	}
	saved := identity
	saved.Contents = append([]string(nil), identity.Contents...)
	return UpdateBranchMeta(sessionPath, false, func(m *BranchMeta) error {
		m.TodoBatch = &saved
		return nil
	})
}

// CloseTodoBatchIdentity records the user's close for the batch with this id,
// leaving any other batch alone.
func CloseTodoBatchIdentity(sessionPath, batchID string) error {
	batchID = strings.TrimSpace(batchID)
	if strings.TrimSpace(sessionPath) == "" || batchID == "" {
		return nil
	}
	return UpdateBranchMeta(sessionPath, false, func(m *BranchMeta) error {
		if m.TodoBatch == nil || m.TodoBatch.ID != batchID {
			return nil
		}
		m.TodoBatch.Closed = true
		return nil
	})
}
