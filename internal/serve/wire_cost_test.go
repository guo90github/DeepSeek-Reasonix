package serve

import (
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/eventwire"
)

// A remote client reads cost from this ledger, and the embedded host pushes its
// frames only through EmitWire: without accounting there, /status answered
// "no_usage" for every session while the desktop's own telemetry still showed a
// cost — the phone rendered 0 and looked like it was showing fake data.
func TestEmitWireAccountsUsageCost(t *testing.T) {
	b := NewBroadcaster()
	const path = `C:\projects\demo\sessions\a.jsonl`
	b.EmitWire(eventwire.Event{
		Kind:        "usage",
		SessionPath: path,
		Usage: &eventwire.Usage{
			PromptTokens: 1000, CompletionTokens: 10, CacheHitTokens: 900, CacheMissTokens: 100,
			CostQuote: &billing.CostQuote{
				Original:     billing.Money{Amount: "123", Currency: "CNY"},
				CostComplete: true,
			},
		},
	})
	quote := b.SessionCostQuoteFor(path)
	if quote.Original.Amount == "" || quote.Original.Amount == "0" {
		t.Fatalf("wire usage event did not reach the session ledger: %+v", quote)
	}
	if other := b.SessionCostQuoteFor(`C:\projects\demo\sessions\b.jsonl`); other.Original.Amount != "" && other.Original.Amount != "0" {
		t.Fatalf("accounting leaked into another session: %+v", other)
	}
}
