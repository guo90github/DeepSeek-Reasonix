package control

import (
	"fmt"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/sessioninbox"
)

// inboxStaleAlertDelay is how long a queued line may sit undispatched before the
// host says so out loud. It is a variable so a test can collapse the wait
// without a fake clock.
var inboxStaleAlertDelay = 10 * time.Minute

// armStaleQueueAlert schedules one check for work the dispatcher left queued.
// The 2026-09-27 stall ran five hours because a held queue and an idle session
// looked identical from outside: nothing ever said a line was still waiting.
func (c *Controller) armStaleQueueAlert() {
	c.inbox.mu.Lock()
	if c.inbox.closed || c.inbox.staleAlertArmed {
		c.inbox.mu.Unlock()
		return
	}
	c.inbox.staleAlertArmed = true
	schedule := c.inbox.scheduleStaleAlert
	c.inbox.mu.Unlock()
	check := func() { c.checkStaleQueue() }
	if schedule != nil {
		schedule(inboxStaleAlertDelay, check)
		return
	}
	time.AfterFunc(inboxStaleAlertDelay, check)
}

// checkStaleQueue reports a line that waited past the alert delay, and which of
// the two human-action situations it is in: a paused queue, or a start that
// keeps failing. A queue that is merely busy says nothing — it is progressing.
func (c *Controller) checkStaleQueue() {
	c.inbox.mu.Lock()
	c.inbox.staleAlertArmed = false
	closed := c.inbox.closed
	c.inbox.mu.Unlock()
	if closed {
		return
	}
	gate := c.inboxDispatchGate()
	if gate != sessioninbox.GatePaused && gate != sessioninbox.GateStartFailed {
		return
	}
	oldest, ok := c.oldestQueuedInboxItem()
	if !ok {
		return
	}
	waited := time.Since(oldest.CreatedAt)
	if waited < inboxStaleAlertDelay || c.staleAlertAlreadySent(oldest.ID) {
		return
	}
	text := fmt.Sprintf(i18n.M.InboxStalePausedFmt, staleWaitText(waited))
	if gate == sessioninbox.GateStartFailed {
		// The failed start is only actionable with its cause attached.
		text = fmt.Sprintf(i18n.M.InboxStaleStartFailedFmt, staleWaitText(waited), c.inboxStartFailure())
	}
	c.sink.Emit(event.Event{
		Kind: event.Notice, Level: event.LevelWarn,
		Text:   text,
		Detail: "gate=" + gate + " item=" + oldest.ID,
	})
}

// staleAlertAlreadySent reports whether this line was already announced, and
// records it otherwise: one line gets one sentence, however long it waits.
func (c *Controller) staleAlertAlreadySent(itemID string) bool {
	c.inbox.mu.Lock()
	defer c.inbox.mu.Unlock()
	if c.inbox.staleAlertedItem == itemID {
		return true
	}
	c.inbox.staleAlertedItem = itemID
	return false
}

func (c *Controller) oldestQueuedInboxItem() (sessioninbox.InboxItemMeta, bool) {
	st, err := c.ensureInbox()
	if err != nil {
		return sessioninbox.InboxItemMeta{}, false
	}
	var oldest sessioninbox.InboxItemMeta
	found := false
	for _, item := range st.CachedSnapshot().Items {
		if item.State != sessioninbox.StateQueued {
			continue
		}
		if !found || item.CreatedAt.Before(oldest.CreatedAt) {
			oldest, found = item, true
		}
	}
	return oldest, found
}

// staleWaitText renders a wait for a person: minutes under an hour, otherwise
// hours and minutes.
func staleWaitText(d time.Duration) string {
	minutes := int(d.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh%dm", minutes/60, minutes%60)
}
