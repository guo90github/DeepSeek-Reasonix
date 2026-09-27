package control

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"reasonix/internal/sessioninbox"
)

// inboxDispatchRetryDelays paces re-attempts after a transient admission or
// Store failure: short, because those either clear at once or are permanent.
var inboxDispatchRetryDelays = [...]time.Duration{
	50 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond,
}

// inboxDispatchDeferDelays paces re-attempts while the host has not published
// this runtime. The host kicks again when it publishes, so this ladder only
// covers the window where that kick never arrives; expiry leaves the item
// queued for the host exactly as before.
var inboxDispatchDeferDelays = [...]time.Duration{
	1 * time.Second, 3 * time.Second, 10 * time.Second, 30 * time.Second,
}

// ErrInboxRuntimeUnpublished means the host owns the next dispatch kick:
// either this runtime is a candidate or it was replaced before admission.
var ErrInboxRuntimeUnpublished = errors.New("inbox runtime is not published")

// InboxDispatchRefusal is a host's own answer for why its publication hook did
// not publish this runtime: Reason is the sentence a sender relays verbatim,
// Resumable says whether another wake could still lift the item, and Err keeps
// the machine-readable cause (errors.Is still matches it). A host that cannot
// name a reason returns the bare error instead.
type InboxDispatchRefusal struct {
	Reason    string
	Resumable bool
	Err       error
}

func (e *InboxDispatchRefusal) Error() string {
	if e == nil {
		return ""
	}
	if reason := strings.TrimSpace(e.Reason); reason != "" {
		return reason
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "inbox dispatch was refused"
}

func (e *InboxDispatchRefusal) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NotifyInboxRuntimeReady is called after a host publishes a complete runtime.
func (c *Controller) NotifyInboxRuntimeReady() {
	c.resetInboxDispatchRetries()
	c.maybeDispatchInbox()
}

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
	// inboxDispatchDeferred means the host has not published this runtime yet:
	// the item stays queued and inboxDispatchDeferDelays re-attempts admission.
	inboxDispatchDeferred
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
		case inboxDispatchDeferred:
			c.scheduleInboxDispatchDefer()
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
		// An empty queue has nothing to explain: drop a failure recorded for a
		// line that is no longer waiting, so it cannot answer for a later one.
		c.clearInboxStartFailure()
		return inboxDispatchIdle
	}
	if beforeSubmit != nil {
		if err := beforeSubmit(meta.ID); err != nil {
			slog.Warn("controller: inbox dispatch hook", "err", err, "id", meta.ID)
			c.noteInboxStartFailure(err)
			return inboxDispatchRetry
		}
	}
	receipt, err := c.TrySubmitInboxItem(meta.ID)
	if err != nil {
		if errors.Is(err, ErrInboxRuntimeUnpublished) {
			return inboxDispatchDeferred
		}
		if errors.Is(err, ErrTurnRunning) {
			return inboxDispatchIdle
		}
		slog.Warn("controller: dispatch inbox item", "err", err, "id", meta.ID)
		c.noteInboxStartFailure(err)
		return inboxDispatchRetry
	}
	if receipt.Disposition == sessioninbox.DispositionStarted {
		c.clearInboxStartFailure()
		return inboxDispatchStarted
	}
	// A competing turn or rotation owns the next kick when its gate releases.
	return inboxDispatchIdle
}

// noteInboxStartFailure records why the last attempt to start a queued turn
// failed, so a line that is still queued stops looking like an untouched queue.
func (c *Controller) noteInboxStartFailure(err error) {
	if err == nil {
		return
	}
	c.inbox.mu.Lock()
	c.inbox.startFailure = err.Error()
	c.inbox.mu.Unlock()
}

func (c *Controller) clearInboxStartFailure() {
	c.inbox.mu.Lock()
	c.inbox.startFailure = ""
	c.inbox.mu.Unlock()
}

// inboxStartFailure reads the recorded start failure under the inbox lock.
func (c *Controller) inboxStartFailure() string {
	c.inbox.mu.Lock()
	defer c.inbox.mu.Unlock()
	return c.inbox.startFailure
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
	// The queue is open and nothing is holding it: if the last start attempt
	// failed, that failure is the reason this line is still here.
	if c.inboxStartFailure() != "" {
		return sessioninbox.GateStartFailed
	}
	return ""
}

// withDispatchGate annotates a receipt whose item is still queued with the gate
// holding it, the host's sentence for it, and whether anything a sender can do
// would move it: the disposition alone only says "queued".
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
	rec.State = meta.State
	rec.GateReason = sessioninbox.GateReasonText(gate)
	rec.PendingPrompt = sessioninbox.GateWaitsForUser(gate)
	resumable := sessioninbox.GateResumable(gate)
	if gate == sessioninbox.GateHostDispatch {
		// The host knew why it did not publish; its own answer beats the gate
		// template, which only says the answer is invisible from here.
		if reason, hostResumable, refused := c.inboxHostRefusalFor(rec.ItemID); refused {
			rec.GateReason = reason
			resumable = hostResumable
		}
		if c.inboxDeferLadderExhausted() {
			retryable := false
			rec.Retryable = &retryable
			rec.RetryReason = inboxDeferExhaustedReason
		}
	}
	rec.Resumable = &resumable
	return rec
}

// inboxDeferExhaustedReason is the sentence for a host that let every rung of
// the defer ladder pass: this side has nothing left to try.
const inboxDeferExhaustedReason = "宿主这一侧四次退避（1s/3s/10s/30s）都没有放行，重试阶梯已经走完；" +
	"再投一条只会拿到同样的答案，等宿主发布运行时（例如重开那个会话的标签页）才会放行。"

// inboxDeferLadderExhausted reports whether the host let the whole defer ladder
// pass without publishing, so re-posting the same wake cannot change anything.
func (c *Controller) inboxDeferLadderExhausted() bool {
	c.inbox.mu.Lock()
	defer c.inbox.mu.Unlock()
	return c.inbox.dispatchDeferAttempts >= len(inboxDispatchDeferDelays)
}

// inboxHostRefusal is one host answer: the sentence a sender relays verbatim,
// and whether another wake could still lift the item it refused.
type inboxHostRefusal struct {
	itemID    string
	reason    string
	resumable bool
}

// noteInboxHostAnswer keeps the host's reason for the item it just refused, and
// drops a stale one once the host lets that item through or names no reason.
func (c *Controller) noteInboxHostAnswer(itemID string, err error) {
	reason, resumable := "", false
	var refusal *InboxDispatchRefusal
	if errors.As(err, &refusal) {
		reason, resumable = strings.TrimSpace(refusal.Reason), refusal.Resumable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if reason == "" {
		if current := c.modelSettings.inboxHostRefusal; current != nil && current.itemID == itemID {
			c.modelSettings.inboxHostRefusal = nil
		}
		return
	}
	c.modelSettings.inboxHostRefusal = &inboxHostRefusal{itemID: itemID, reason: reason, resumable: resumable}
}

func (c *Controller) inboxHostRefusalFor(itemID string) (string, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	refusal := c.modelSettings.inboxHostRefusal
	if refusal == nil || refusal.itemID != itemID {
		return "", false, false
	}
	return refusal.reason, refusal.resumable, true
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
	c.scheduleInboxDispatchAttempt(&c.inbox.dispatchRetryAttempts, inboxDispatchRetryDelays[:])
}

// scheduleInboxDispatchDefer re-attempts admission while the host has not
// published this runtime. Running out of ladder leaves the item queued for the
// host, which is what happened on the first attempt before.
func (c *Controller) scheduleInboxDispatchDefer() {
	c.scheduleInboxDispatchAttempt(&c.inbox.dispatchDeferAttempts, inboxDispatchDeferDelays[:])
}

// scheduleInboxDispatchAttempt arms the next attempt from one ladder, sharing a
// single in-flight flag so a queued item never holds two timers. The ladders are
// bounded: a persistent condition must not become a hot background loop.
func (c *Controller) scheduleInboxDispatchAttempt(attempts *int, delays []time.Duration) {
	c.inbox.mu.Lock()
	if c.inbox.dispatchRetryScheduled || *attempts >= len(delays) {
		c.inbox.mu.Unlock()
		return
	}
	delay := delays[*attempts]
	*attempts++
	c.inbox.dispatchRetryScheduled = true
	schedule := c.inbox.scheduleDispatchRetry
	c.inbox.mu.Unlock()

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
	c.inbox.dispatchDeferAttempts = 0
	c.inbox.mu.Unlock()
}
