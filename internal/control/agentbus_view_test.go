package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// agentBusViewLine returns the rendered row for one node, or "" when it is absent.
func agentBusViewLine(rendered, id string) string {
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, "node id="+id+" ") {
			return line
		}
	}
	return ""
}

// A turn consumes the delta and advances the cursor, which used to leave a reader
// with counters and no rows — nothing new to show and no way to ask again. A
// participant that still has work has to be able to re-read it, which is what the
// peek does now: an empty delta falls back to the current set, read from the start.
func TestAgentBusViewReReadsAfterTheTurnConsumedTheDelta(t *testing.T) {
	ctx := context.Background()
	c := newAgentBusTalkController(t, t.TempDir(), "me")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("work", "me")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("work", "me", "dep")); err != nil {
		t.Fatalf("require: %v", err)
	}

	if first := c.agentBusTurnBlock(); first == "" {
		t.Fatal("the first turn must carry the delta")
	}
	if second := c.agentBusTurnBlock(); second != "" {
		t.Fatalf("the cursor must suppress a repeat delivery, got %q", second)
	}

	view, ok := c.AgentBusView(time.Now().UTC())
	if !ok {
		t.Fatal("this participant is on a board")
	}
	if len(view.Lines) == 0 {
		t.Fatalf("a participant with work must still be able to re-read it:\n%s", view.Render())
	}
	if view.Cursor != 0 {
		t.Fatalf("a re-read has to say where it read from, got cursor=%d", view.Cursor)
	}
	rendered := view.Render()
	if strings.Contains(rendered, "deps_done") {
		t.Fatalf("the row still carries the old field name:\n%s", rendered)
	}

	// startable means "can run now", not "my deps are done": the container waits on a
	// step (deps_open=1) while that step is the one that can start.
	if line := agentBusViewLine(rendered, "work"); !strings.Contains(line, "state=blocked") ||
		!strings.Contains(line, "deps_open=1") || !strings.Contains(line, "startable=false") {
		t.Fatalf("the waiting container reads wrong: %s", line)
	}
	if line := agentBusViewLine(rendered, "dep"); !strings.Contains(line, "state=open") ||
		!strings.Contains(line, "deps_open=0") || !strings.Contains(line, "startable=true") {
		t.Fatalf("the startable step reads wrong: %s", line)
	}

	// A live claim makes a node not startable, which is what the field name says.
	if _, err := c.ApplyAgentBusOp(ctx, board.Op{Verb: board.VerbClaim, Node: "dep", Actor: "me",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(time.Minute)}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed, _ := c.AgentBusView(time.Now().UTC())
	if line := agentBusViewLine(claimed.Render(), "dep"); !strings.Contains(line, "state=claimed") ||
		!strings.Contains(line, "startable=false") {
		t.Fatalf("a claimed node must read startable=false: %s", line)
	}
}
