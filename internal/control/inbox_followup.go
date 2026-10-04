package control

import (
	"log/slog"
	"strings"

	"reasonix/internal/sessioninbox"
)

// TryEnqueueFollowup durably queues a follow-up and may dispatch if idle.
func (c *Controller) TryEnqueueFollowup(req InboxRequest) (sessioninbox.InboxReceipt, error) {
	req.Intent = sessioninbox.IntentFollowup
	if strings.TrimSpace(req.Source) == AgentBusWakeSource && isGenericAgentBusWakeKey(req.Idempotency) {
		// One generic wake per participant is enough: the injector re-derives the whole work set
		// when that item's turn ends, so a second copy can only repeat it or retract it. A
		// dispatch wake is exempt — each names one assignment (2026-10-05).
		if waiting := c.genericAgentBusWakeWaiting(); waiting != "" {
			return c.withDispatchGate(sessioninbox.InboxReceipt{ItemID: waiting}), nil
		}
	}
	rec, err := c.EnqueueInbox(req)
	if err != nil {
		return rec, err
	}
	if !c.Running() {
		c.maybeDispatchInbox()
	}
	return c.withDispatchGate(rec), nil
}

// isGenericAgentBusWakeKey reports a wake that names a whole work set, as opposed to a dispatch
// wake, which names one assignment.
func isGenericAgentBusWakeKey(key string) bool {
	return strings.HasPrefix(strings.TrimSpace(key), agentBusWakeKeyPrefix)
}

// genericAgentBusWakeWaiting is the one generic wake this session may keep queued, and drops any
// extra copies: the injector re-derives the whole work set when that item's turn ends, so an older
// copy can only deliver a stale list or a retraction (2026-10-05: a drill left 11 queued on one
// session). An empty id means nothing is waiting.
func (c *Controller) genericAgentBusWakeWaiting() string {
	st, err := c.ensureInbox()
	if err != nil {
		return ""
	}
	return genericAgentBusWakeWaiting(st)
}

func genericAgentBusWakeWaiting(st *sessioninbox.Store) string {
	if st == nil {
		return ""
	}
	var keep string
	var extras []string
	for _, item := range st.Snapshot().Items {
		if item.State != sessioninbox.StateQueued || strings.TrimSpace(item.Source) != AgentBusWakeSource {
			continue
		}
		if !isGenericAgentBusWakeKey(item.Idempotency) {
			continue
		}
		if keep == "" {
			keep = item.ID
			continue
		}
		extras = append(extras, item.ID)
	}
	if len(extras) > 0 {
		if err := st.DiscardPendingItemsOwned(extras, AgentBusWakeSource); err != nil {
			slog.Warn("control: could not fold a duplicate agentbus wake", "err", err, "items", len(extras))
		}
	}
	return keep
}
