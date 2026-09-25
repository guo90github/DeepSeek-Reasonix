package control

import (
	"errors"
	"log/slog"
	"time"

	"reasonix/internal/sessioninbox"
)

const maxInboxDispatchRetryAttempts = 3

// ErrInboxRuntimeUnpublished means the host owns the next dispatch kick:
// either this runtime is a candidate or it was replaced before admission.
var ErrInboxRuntimeUnpublished = errors.New("inbox runtime is not published")

// NotifyInboxRuntimeReady is called after a host publishes a complete runtime.
func (c *Controller) NotifyInboxRuntimeReady() { c.maybeDispatchInbox() }

func (c *Controller) SetBeforeInboxDispatch(before func(*Controller) (func(), error)) {
	c.mu.Lock()
	c.modelSettings.beforeInboxDispatch = before
	c.mu.Unlock()
}

type inboxDispatchResult int

const (
	inboxDispatchIdle inboxDispatchResult = iota
	inboxDispatchStarted
	inboxDispatchRetry
)

// endRotation releases the admission gate and republishes durable queue work.
func (c *Controller) endRotation() {
	c.mu.Lock()
	c.rotating = false
	c.mu.Unlock()
	c.maybeDispatchInbox()
}

// maybeDispatchInbox is a level-triggered kick, not a one-shot edge. Every
// caller publishes pending work before checking whether a dispatcher is live.
// The active dispatcher clears dispatching only while holding the same lock
// after observing no pending kick, so a completion, rotation release, or steer
// rejection can never disappear in the handoff window.
func (c *Controller) maybeDispatchInbox() {
	c.inbox.mu.Lock()
	if c.inbox.closed {
		c.inbox.mu.Unlock()
		return
	}
	c.inbox.dispatchPending = true
	if c.inbox.dispatching {
		c.inbox.mu.Unlock()
		return
	}
	c.inbox.dispatching = true
	c.inbox.mu.Unlock()
	c.mu.Lock()
	hostAdmission := c.modelSettings.beforeInboxDispatch != nil
	c.mu.Unlock()
	if hostAdmission {
		// Enqueue/resume can be called with the host's publication lock held.
		// Never synchronously reenter that lock through its admission callback.
		c.autosaveWG.Go(c.drainInboxDispatch)
		return
	}
	c.drainInboxDispatch()
}

func (c *Controller) drainInboxDispatch() {
	for {
		c.inbox.mu.Lock()
		if !c.inbox.dispatchPending {
			c.inbox.dispatching = false
			c.inbox.mu.Unlock()
			return
		}
		c.inbox.dispatchPending = false
		c.inbox.mu.Unlock()

		switch c.dispatchInboxOnce() {
		case inboxDispatchRetry:
			c.scheduleInboxDispatchRetry()
		case inboxDispatchStarted, inboxDispatchIdle:
			c.resetInboxDispatchRetries()
		}
	}
}

// dispatchInboxOnce admits one FIFO item when every runtime gate is open. A
// started turn owns the next kick through finishGuardedTurn; this method never
// loops over multiple items while that turn is active.
func (c *Controller) dispatchInboxOnce() inboxDispatchResult {
	if c.PendingPrompt() {
		return inboxDispatchIdle
	}
	c.mu.Lock()
	busy := c.running || c.finishing || c.rotating || c.closed
	c.mu.Unlock()
	if busy {
		return inboxDispatchIdle
	}
	// Controllers without persistence cannot own a durable inbox. Rotation and
	// turn-completion hooks are shared with those controllers, so treat the
	// missing path as an empty queue instead of retrying a permanent condition.
	if c.SessionPath() == "" {
		return inboxDispatchIdle
	}
	meta, ok, err := c.nextInboxDispatchItem()
	if err != nil {
		slog.Warn("controller: open inbox for dispatch", "err", err)
		return inboxDispatchRetry
	}
	c.inbox.mu.Lock()
	beforeSubmit := c.inbox.beforeDispatchSubmit
	c.inbox.mu.Unlock()
	if !ok {
		return inboxDispatchIdle
	}
	if beforeSubmit != nil {
		if err := beforeSubmit(meta.ID); err != nil {
			slog.Warn("controller: inbox dispatch hook", "err", err, "id", meta.ID)
			return inboxDispatchRetry
		}
	}
	receipt, err := c.TrySubmitInboxItem(meta.ID)
	if err != nil {
		if errors.Is(err, ErrInboxRuntimeUnpublished) || errors.Is(err, ErrTurnRunning) {
			return inboxDispatchIdle
		}
		slog.Warn("controller: dispatch inbox item", "err", err, "id", meta.ID)
		return inboxDispatchRetry
	}
	if receipt.Disposition == sessioninbox.DispositionStarted {
		return inboxDispatchStarted
	}
	// A competing turn or rotation owns the next kick when its gate releases.
	return inboxDispatchIdle
}

// inboxDispatchGate names the gate that currently holds queued work, checked in
// the same order as dispatchInboxOnce so a receipt can say what it waits on
// instead of only that it is queued.
func (c *Controller) inboxDispatchGate() string {
	if c.PendingPrompt() {
		return sessioninbox.GateAwaitingAnswer
	}
	c.mu.Lock()
	switch {
	case c.closed:
		c.mu.Unlock()
		return sessioninbox.GateClosed
	case c.rotating:
		c.mu.Unlock()
		return sessioninbox.GateRotating
	case c.running:
		c.mu.Unlock()
		return sessioninbox.GateTurnRunning
	case c.finishing:
		c.mu.Unlock()
		return sessioninbox.GateTurnFinishing
	}
	hostHook := c.modelSettings.beforeInboxDispatch != nil
	c.mu.Unlock()
	// Only a dispatcher reaches this branch without a session file: a caller
	// that names no session is refused before admission and holds no receipt.
	if c.SessionPath() == "" {
		return sessioninbox.GateNoSessionPath
	}
	st, err := c.ensureInbox()
	if err != nil {
		return ""
	}
	snap := st.CachedSnapshot()
	if snap.Paused {
		return sessioninbox.GatePaused
	}
	if snap.Readonly {
		return sessioninbox.GateReadonly
	}
	if hostHook {
		// The host's publication hook owns the next kick; its answer is only
		// visible to the dispatcher, never to the receipt.
		return sessioninbox.GateHostDispatch
	}
	return ""
}

// withDispatchGate annotates a receipt whose item is still queued with the gate
// holding it and the host's sentence for it: the disposition only says "queued".
func (c *Controller) withDispatchGate(rec sessioninbox.InboxReceipt) sessioninbox.InboxReceipt {
	if rec.Gate != "" || rec.ItemID == "" {
		return rec
	}
	st, err := c.ensureInbox()
	if err != nil {
		return rec
	}
	meta, _, err := st.ReadItem(rec.ItemID)
	if err != nil || (meta.State != sessioninbox.StateQueued && meta.State != sessioninbox.StateUncertain) {
		return rec
	}
	gate := c.inboxDispatchGate()
	rec.Gate = gate
	rec.GateReason = sessioninbox.GateReasonText(gate)
	rec.PendingPrompt = sessioninbox.GateWaitsForUser(gate)
	return rec
}

func (c *Controller) nextInboxDispatchItem() (sessioninbox.InboxItemMeta, bool, error) {
	c.inbox.scanMu.Lock()
	defer c.inbox.scanMu.Unlock()
	c.inbox.mu.Lock()
	closed := c.inbox.closed
	afterScan := c.inbox.afterDispatchScan
	c.inbox.mu.Unlock()
	if closed {
		return sessioninbox.InboxItemMeta{}, false, nil
	}
	st, err := c.ensureInbox()
	if err != nil {
		return sessioninbox.InboxItemMeta{}, false, err
	}
	// NextQueued refreshes disk state and may create its transaction-lock
	// directory. Keep that access inside the same shutdown boundary as Open.
	meta, ok := st.NextQueued()
	if afterScan != nil {
		afterScan(ok)
	}
	return meta, ok, nil
}

func (c *Controller) scheduleInboxDispatchRetry() {
	c.inbox.mu.Lock()
	if c.inbox.dispatchRetryScheduled || c.inbox.dispatchRetryAttempts >= maxInboxDispatchRetryAttempts {
		c.inbox.mu.Unlock()
		return
	}
	attempt := c.inbox.dispatchRetryAttempts
	c.inbox.dispatchRetryAttempts++
	c.inbox.dispatchRetryScheduled = true
	schedule := c.inbox.scheduleDispatchRetry
	c.inbox.mu.Unlock()

	delay := [...]time.Duration{50 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond}[attempt]
	retry := func() {
		c.inbox.mu.Lock()
		c.inbox.dispatchRetryScheduled = false
		c.inbox.mu.Unlock()
		c.maybeDispatchInbox()
	}
	if schedule != nil {
		schedule(delay, retry)
		return
	}
	time.AfterFunc(delay, retry)
}

func (c *Controller) resetInboxDispatchRetries() {
	c.inbox.mu.Lock()
	c.inbox.dispatchRetryAttempts = 0
	c.inbox.mu.Unlock()
}
