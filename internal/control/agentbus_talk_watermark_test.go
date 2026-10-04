package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
)

// Re-enrolling on the same board must not make the session read its whole talk history again.
// A rebuilt state starts its watermarks at zero, and on the real board one session then got a
// block re-listing lines it had already been given (26 lines / 25362 bytes, F26, 2026-10-05).
func TestReEnrollingKeepsTheTalkWatermark(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	me := newAgentBusTalkController(t, dir, "bob")

	line, err := me.appendTalk(ctx, agentbus.TalkLine{
		Topic: "t", Kind: agentbus.TalkSay, To: "bob", Text: "hands off the lease probe", At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	first := me.agentBusTalkBlock()
	if !strings.Contains(first, line.Text) {
		t.Fatalf("the first block does not carry the line addressed to bob:\n%s", first)
	}

	me.SetAgentBus(dir, "bob")
	second := me.agentBusTalkBlock()
	if strings.Contains(second, line.Text) {
		t.Fatalf("re-enrolling re-delivered an already-delivered line:\n%s", second)
	}
}

// A different board has unrelated sequence numbers, so its reading starts from the beginning.
func TestEnrollingOnAnotherBoardStartsItsOwnWatermark(t *testing.T) {
	ctx := context.Background()
	me := newAgentBusTalkController(t, t.TempDir(), "bob")

	other := t.TempDir()
	if _, err := me.appendTalk(ctx, agentbus.TalkLine{}); err == nil {
		t.Fatal("a line with no topic must be refused")
	}
	me.SetAgentBus(other, "bob")
	if got := me.agentBusTalkBlock(); got != "" {
		t.Fatalf("an empty board produced %q, want nothing", got)
	}
}
