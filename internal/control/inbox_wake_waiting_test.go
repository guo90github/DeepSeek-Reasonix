package control

import (
	"testing"

	"reasonix/internal/agentbus"
)

// F7 第 2 条: a board change used to mint a fresh generic wake per participant, and a busy session
// could not consume them, so they stacked (2026-10-05: 11 queued on the orchestrator, each one
// spending its own turn at the end). The wake a session already holds is enough: the injector
// re-derives the whole work set when that item's turn ends, so the fix is to answer with the item
// that is already waiting instead of queueing another.
func TestASecondGenericWakeWaitsInsteadOfStacking(t *testing.T) {
	c, _, _ := newInboxDispatchController(t)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	wake := func(key string) string {
		t.Helper()
		rec, err := c.TryEnqueueFollowup(InboxRequest{
			Submit: key, Source: AgentBusWakeSource, Idempotency: key,
		})
		if err != nil {
			t.Fatalf("enqueue %s: %v", key, err)
		}
		return rec.ItemID
	}

	first := wake("agentbus-wake:default/bob/aaaa")
	if first == "" {
		t.Fatal("the first wake must be queued as its own item")
	}
	for _, key := range []string{"agentbus-wake:default/bob/bbbb", "agentbus-wake:default/bob/cccc"} {
		if got := wake(key); got != first {
			t.Fatalf("the %s wake answered with item %q, want the one already waiting (%q)", key, got, first)
		}
	}
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	if got := queuedInboxItems(t, st); got != 1 {
		t.Fatalf("queued = %d, want one wake waiting however many board changes arrived", got)
	}

	// A dispatch wake names one assignment and the injector does not re-derive it, so it must be
	// queued even though a generic wake is already waiting.
	wake(agentbus.DispatchKey("default", "schema"))
	if got := queuedInboxItems(t, st); got != 2 {
		t.Fatalf("queued = %d, want the dispatch wake kept alongside the generic one", got)
	}
}
