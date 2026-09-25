package sessioninbox

import "time"

// LookupReceipt reads the existing bounded idempotency records without
// creating or replaying a write. Used after an uncertain transport outcome.
func (s *Store) LookupReceipt(key string) (InboxReceipt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" {
		return InboxReceipt{}, false
	}
	if id, ok := s.man.Idempotency[key]; ok {
		if meta, found := s.man.item(id); found {
			return s.receiptForItemLocked(meta), true
		}
	}
	if receipt, ok := s.man.Receipts[key]; ok && time.Since(receipt.CompletedAt) <= idempotencyReceiptTTL {
		return InboxReceipt{
			ItemID: receipt.ItemID, Disposition: DispositionIdempotentHit,
			Paused: s.man.Paused, Capacity: s.snapshotLocked().Capacity, Idempotent: true,
		}, true
	}
	return InboxReceipt{}, false
}

// receiptForItemLocked reads the queue as it is now: a sender re-asking about a
// wake needs the item's current place and lifecycle, not the pair it saw on the
// first answer.
func (s *Store) receiptForItemLocked(meta InboxItemMeta) InboxReceipt {
	return InboxReceipt{
		ItemID: meta.ID, Disposition: DispositionIdempotentHit,
		Position: s.man.positionOf(meta.ID), Paused: s.man.Paused,
		Capacity: s.snapshotLocked().Capacity, Idempotent: true,
		State: meta.State,
	}
}

func (s *Store) idempotentReceiptLocked(key, requestHash string) (InboxReceipt, bool, error) {
	if key == "" {
		return InboxReceipt{}, false, nil
	}
	if id, ok := s.man.Idempotency[key]; ok {
		if item, found := s.man.item(id); found {
			if previous := s.man.IdempotencyHashes[key]; previous != "" && previous != requestHash {
				return InboxReceipt{}, false, ErrIdempotencyConflict
			}
			return InboxReceipt{
				ItemID: item.ID, Disposition: DispositionIdempotentHit,
				Position: s.man.positionOf(item.ID), Paused: s.man.Paused,
				Capacity: s.snapshotLocked().Capacity, Idempotent: true,
				State: item.State,
			}, true, nil
		}
	}
	receipt, ok := s.man.Receipts[key]
	if !ok || time.Since(receipt.CompletedAt) > idempotencyReceiptTTL {
		return InboxReceipt{}, false, nil
	}
	if receipt.RequestHash != requestHash {
		return InboxReceipt{}, false, ErrIdempotencyConflict
	}
	return InboxReceipt{
		ItemID: receipt.ItemID, Disposition: DispositionIdempotentHit,
		Paused: s.man.Paused, Capacity: s.snapshotLocked().Capacity, Idempotent: true,
	}, true, nil
}

func (s *Store) idempotentAliasReplayLocked(key, requestHash, itemID string) (bool, error) {
	if key == "" {
		return false, nil
	}
	if existingID, ok := s.man.Idempotency[key]; ok {
		if existingHash := s.man.IdempotencyHashes[key]; existingHash != "" && existingHash != requestHash {
			return false, ErrIdempotencyConflict
		}
		if existingID != itemID {
			return false, ErrIdempotencyConflict
		}
		return true, nil
	}
	receipt, ok := s.man.Receipts[key]
	if !ok || time.Since(receipt.CompletedAt) > idempotencyReceiptTTL {
		return false, nil
	}
	if receipt.RequestHash != requestHash || receipt.ItemID != itemID {
		return false, ErrIdempotencyConflict
	}
	return true, nil
}

func bindIdempotency(m *manifest, key, itemID, requestHash string) {
	if key == "" {
		return
	}
	if m.Idempotency == nil {
		m.Idempotency = map[string]string{}
	}
	if m.IdempotencyHashes == nil {
		m.IdempotencyHashes = map[string]string{}
	}
	m.Idempotency[key] = itemID
	m.IdempotencyHashes[key] = requestHash
}
