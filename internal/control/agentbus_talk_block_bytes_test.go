package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
)

// The line cap bounds how many lines arrive and the shortening rule bounds one line, but neither
// bounds the block: 50 shortened lines still reached the turn. The budget stops the block and
// leaves the rest above the watermark, so the next block sends them (2026-10-05).
func TestTheTalkBlockStopsAtItsByteBudgetWithoutLosingLines(t *testing.T) {
	ctx := context.Background()
	me := newAgentBusTalkController(t, t.TempDir(), "bob")

	// A shortened line is a few hundred bytes, so this is several budgets' worth of talk.
	const total = 40
	payload := strings.Repeat("y", 4*1024)
	for range total {
		if _, err := me.appendTalk(ctx, agentbus.TalkLine{
			Topic: "t", Kind: agentbus.TalkSay, To: "bob", Text: payload, At: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	block := me.agentBusTalkBlock()
	if len(block) > agentBusTalkMaxBytes+talkCountersReserve {
		t.Fatalf("the block carried %d bytes, over its %d budget", len(block), agentBusTalkMaxBytes)
	}
	if !strings.Contains(block, "truncated=true") {
		t.Fatalf("the block does not report what it left for the next one")
	}
	carried := strings.Count(block, "\nline seq=")
	if carried == 0 || carried >= total {
		t.Fatalf("carried %d of %d lines, want some but not all", carried, total)
	}

	// Nothing the budget left out may be lost: the blocks that follow have to carry it.
	seen := carried
	for i := 0; i < total && seen < total; i++ {
		next := me.agentBusTalkBlock()
		if next == "" {
			break
		}
		seen += strings.Count(next, "\nline seq=")
	}
	if seen != total {
		t.Fatalf("the blocks carried %d of %d lines in total: the budget lost some", seen, total)
	}
}
