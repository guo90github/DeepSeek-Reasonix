package control

import (
	"log/slog"
	"strings"
	"time"

	"reasonix/internal/sessioninbox"
)

// InboxRoomLine is what this session holds for one room line, addressed by the seq
// the room prints: the durable state plus — while it is queued — the gate holding
// it and the host's own sentence. The push route has no reply channel, so this is
// how a sender asks the host what became of the line it pushed.
type InboxRoomLine struct {
	ItemID    string `json:"itemId"`
	State     string `json:"state"`
	Source    string `json:"source,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Gate      string `json:"gate,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Resumable bool   `json:"resumable,omitempty"`
	// Refused says the host answered for this line at all. It is what makes
	// Resumable meaningful: an absent Resumable means "no" only next to a
	// refusal, and says nothing about a line nobody refused.
	Refused bool `json:"refused,omitempty"`
	// QueuedForMs answers only "how long", never "why it has not run" — that is
	// the gate plus the host's own sentence.
	QueuedForMs int64 `json:"queuedForMs,omitempty"`
	// Settled is how a line that already left the queue ended, so "ran and
	// finished" does not have to be read as "never took it in".
	Settled   string `json:"settled,omitempty"`
	SettledAt string `json:"settledAt,omitempty"`
}

// settledRoomLine is one remembered ending, bounded in count.
type settledRoomLine struct {
	disposition string
	at          time.Time
}

// roomLineSettledLimit keeps the ledger small: a line older than this many endings
// falls back to the two-question path (queue plus receipt by idempotency key).
const roomLineSettledLimit = 64

// noteRoomLineSettled remembers how a room line left the queue. The item that
// carried the seq is gone by then, so this is the only place the pair survives.
func (c *Controller) noteRoomLineSettled(seq int64, disposition string) {
	if seq <= 0 || strings.TrimSpace(disposition) == "" {
		return
	}
	c.inbox.mu.Lock()
	defer c.inbox.mu.Unlock()
	if c.inbox.settledRoomLines == nil {
		c.inbox.settledRoomLines = map[int64]settledRoomLine{}
	}
	c.inbox.settledRoomLines[seq] = settledRoomLine{disposition: disposition, at: time.Now().UTC()}
	for len(c.inbox.settledRoomLines) > roomLineSettledLimit {
		oldestSeq, oldest := int64(0), time.Time{}
		for key, line := range c.inbox.settledRoomLines {
			if oldest.IsZero() || line.at.Before(oldest) {
				oldestSeq, oldest = key, line.at
			}
		}
		delete(c.inbox.settledRoomLines, oldestSeq)
	}
}

// roomSeqsOf reads the room lines carried by these items, before they leave: after
// removal only the receipt still knows the pair. It reads the store, never
// InboxSnapshot — a snapshot recovers orphans, and a cancel must not change state.
func (c *Controller) roomSeqsOf(st *sessioninbox.Store, itemIDs []string) map[string]int64 {
	wanted := make(map[string]bool, len(itemIDs))
	for _, id := range itemIDs {
		wanted[id] = true
	}
	seqs := make(map[string]int64, len(itemIDs))
	if st == nil {
		return seqs
	}
	for _, item := range st.Snapshot().Items {
		if wanted[item.ID] && item.Room != nil && item.Room.Seq > 0 {
			seqs[item.ID] = item.Room.Seq
		}
	}
	return seqs
}

// noteRoomLinesGone remembers the endings of room lines removed without running:
// "discarded" is a cancelled pending item, "deleted" one removed outright. Both
// words are the store's own, so this ledger and the durable receipt agree.
func (c *Controller) noteRoomLinesGone(seqs map[string]int64, disposition string) {
	for _, seq := range seqs {
		c.noteRoomLineSettled(seq, disposition)
	}
}

func (c *Controller) settledRoomLineFor(seq int64) (settledRoomLine, bool) {
	c.inbox.mu.Lock()
	defer c.inbox.mu.Unlock()
	line, ok := c.inbox.settledRoomLines[seq]
	return line, ok
}

func (c *Controller) InboxSnapshot() sessioninbox.InboxSnapshot {
	st, err := c.ensureInbox()
	if err != nil {
		return sessioninbox.InboxSnapshot{}
	}
	if recovered, recoverErr := st.RecoverOrphanedInFlightOwnedBy(c.inbox.ownsItem); recoverErr != nil {
		slog.Warn("controller: recover orphaned inbox items", "err", recoverErr)
	} else if recovered > 0 {
		sessioninbox.NoteRecovered(recovered)
	}
	c.inbox.mu.Lock()
	beforeSnapshotRead := c.inbox.beforeSnapshotRead
	c.inbox.mu.Unlock()
	if beforeSnapshotRead != nil {
		beforeSnapshotRead()
	}
	return st.Snapshot()
}

// InboxDispatchWait says why a queued item is not running yet: the gate holding
// this queue, plus — when the host refused it — the host's own sentence and
// whether another wake could still lift it. It reads, never mutates, so a
// surface that shows it cannot change what the queue does.
func (c *Controller) InboxDispatchWait(itemID string) (gate, reason string, resumable, ok bool) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return "", "", false, false
	}
	gate = c.inboxDispatchGate()
	reason, resumable, refused := c.inboxHostRefusalFor(itemID)
	return gate, reason, resumable, gate != "" || refused
}

// InboxRoomLineFor finds the item carrying this room seq. "Not found" is an
// answer of its own: this session's queue does not hold that line. It does not
// mean the session never took it in — a line that ran is acknowledged and leaves
// the queue, so Settled says how it ended instead.
func (c *Controller) InboxRoomLineFor(seq int64) (InboxRoomLine, bool) {
	if seq <= 0 {
		return InboxRoomLine{}, false
	}
	for _, item := range c.InboxSnapshot().Items {
		if item.Room == nil || item.Room.Seq != seq {
			continue
		}
		line := InboxRoomLine{
			ItemID: item.ID, State: string(item.State),
			Source: item.Source, Preview: item.Preview,
		}
		if item.State == sessioninbox.StateQueued {
			if gate, reason, resumable, ok := c.InboxDispatchWait(item.ID); ok {
				line.Gate, line.Reason, line.Resumable = gate, reason, resumable
			}
			// A refusal without a sentence is not recorded as one, so the pair
			// "refused" and "has a sentence" stay the same fact.
			if _, _, refused := c.inboxHostRefusalFor(item.ID); refused {
				line.Refused = true
			}
			if !item.CreatedAt.IsZero() {
				if waited := time.Since(item.CreatedAt).Milliseconds(); waited > 0 {
					line.QueuedForMs = waited
				}
			}
		}
		return line, true
	}
	// A line that already ran left the queue for good, so the ledger is asked only
	// after the queue says nothing: "not in the queue" stays the honest answer, and
	// Settled says why it is gone.
	if settled, ok := c.settledRoomLineFor(seq); ok {
		return InboxRoomLine{
			Settled:   settled.disposition,
			SettledAt: settled.at.Format(time.RFC3339),
		}, false
	}
	return InboxRoomLine{}, false
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
