package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
)

// One speech can be arbitrarily long and the line cap alone let a single long one through in
// full: a line that names this participant has to arrive shortened, with what it left out
// named, or one message is a context bomb (F7's leftover, 2026-10-05).
func TestATalkLineTooLongToCarryArrivesShortened(t *testing.T) {
	ctx := context.Background()
	me := newAgentBusTalkController(t, t.TempDir(), "bob")

	long := strings.Repeat("x", 8*1024)
	if _, err := me.appendTalk(ctx, agentbus.TalkLine{
		Topic: "t", Kind: agentbus.TalkSay, To: "bob", Text: long, At: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	block := me.agentBusTalkBlock()
	if strings.Contains(block, long) {
		t.Fatalf("the block carried the whole speech (%d bytes)", len(long))
	}
	if !strings.Contains(block, "truncated=true") {
		t.Fatalf("the block does not report that it shortened a line:\n%s", block)
	}
	if !strings.Contains(block, "action=view") {
		t.Fatalf("the shortened line does not say where the rest is:\n%s", block)
	}
	// A shortened line was delivered, so it must not come back on the next block.
	if next := me.agentBusTalkBlock(); strings.Contains(next, "topic=t") {
		t.Fatalf("the shortened line came back:\n%s", next)
	}
}

// A line that fits is carried verbatim, and a block that fits reports no truncation.
func TestATalkLineThatFitsIsCarriedVerbatim(t *testing.T) {
	ctx := context.Background()
	me := newAgentBusTalkController(t, t.TempDir(), "bob")

	if _, err := me.appendTalk(ctx, agentbus.TalkLine{
		Topic: "t", Kind: agentbus.TalkSay, To: "bob", Text: "the short one", At: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	block := me.agentBusTalkBlock()
	if !strings.Contains(block, `text="the short one"`) {
		t.Fatalf("a line that fits was not carried verbatim:\n%s", block)
	}
	if strings.Contains(block, "truncated=true") {
		t.Fatalf("a block that fits reported truncation:\n%s", block)
	}
}
