package control

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
)

// A line the per-turn cap drops is not delivered, so the watermark must not pass it: the old
// order advanced the watermark first and dropped the line afterwards, which retired the
// remainder for good — no later block ever carried it (2026-10-05).
func TestTalkLinesDroppedByTheCapComeBackInTheNextBlock(t *testing.T) {
	ctx := context.Background()
	me := newAgentBusTalkController(t, t.TempDir(), "bob")

	const extra = 3
	total := agentBusTalkMaxLines + extra
	for i := 1; i <= total; i++ {
		if _, err := me.appendTalk(ctx, agentbus.TalkLine{
			Topic: "t", Kind: agentbus.TalkSay, To: "bob",
			Text: fmt.Sprintf("line %d", i), At: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	first := me.agentBusTalkBlock()
	if !strings.Contains(first, "truncated=true") {
		t.Fatalf("the first block should report the cap it hit:\n%s", first)
	}
	if got := strings.Count(first, "\nline seq="); got != agentBusTalkMaxLines {
		t.Fatalf("first block carried %d lines, want the cap (%d)", got, agentBusTalkMaxLines)
	}

	second := me.agentBusTalkBlock()
	if got := strings.Count(second, "\nline seq="); got != extra {
		t.Fatalf("the next block carried %d lines, want the %d the cap dropped:\n%s", got, extra, second)
	}
}
