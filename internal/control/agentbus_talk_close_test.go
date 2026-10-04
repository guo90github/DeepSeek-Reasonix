package control

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus"
)

// I7: a lapsed topic used to need a *writer* to be closed, because the log records the closure
// instead of re-deriving it at read time. With nobody writing, the quiet topic stayed open and
// kept naming an addressee with nothing left to answer — the real machine showed five asks from
// 10-03 still producing "wake target has no route" a day later. The host tick closes it now.
func TestTheHostTickClosesALapsedTopicWithoutAWriter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	c.SetAgentBusTalkLimits(agentbus.TalkLimits{SilenceWindow: time.Millisecond})
	if _, err := c.AgentBusAsk(ctx, "publish-plan", "reviewer", "will you review the plan?"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	_, st, err := c.agentBusTalkSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if topic := st.Topics["publish-plan"]; topic == nil || topic.Closed {
		t.Fatalf("topic = %+v, want it open right after the ask", topic)
	}

	// Nobody writes another talk line: only the tick can notice the silence.
	time.Sleep(5 * time.Millisecond)
	c.AgentBusTick(ctx)

	_, after, err := c.agentBusTalkSnapshot()
	if err != nil {
		t.Fatalf("snapshot after tick: %v", err)
	}
	if topic := after.Topics["publish-plan"]; topic == nil || !topic.Closed {
		t.Fatalf("topic = %+v, want the tick to have recorded the closure", topic)
	}

	// A second tick writes nothing: the closure is a record, and recording it twice would make
	// the log grow on every tick for a topic nobody is talking about.
	log, err := agentbus.OpenTalkLog(dir)
	if err != nil {
		t.Fatalf("open talk log: %v", err)
	}
	_, read, err := log.Read()
	if err != nil {
		t.Fatalf("read talk log: %v", err)
	}
	lines := len(read.Items)
	c.AgentBusTick(ctx)
	_, again, err := log.Read()
	if err != nil {
		t.Fatalf("read talk log again: %v", err)
	}
	if len(again.Items) != lines {
		t.Fatalf("talk log grew from %d to %d lines on a tick that had nothing to close", lines, len(again.Items))
	}
}
