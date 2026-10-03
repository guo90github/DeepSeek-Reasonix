package boot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
	"reasonix/internal/tool/builtin"
)

// The talk surface has to be reachable from the tool the model already has: a participant
// another agent can ask something is the difference between a board work can be handed
// across and one only a human can route. The ask has to land as a record addressed to the
// one participant it names — never as a broadcast.
//
// The other half, that a talk record addressed to you rides your own turn as an
// <agentbus-talk> block, is pinned by internal/control's talk tests; this guard holds the
// shipped write path to producing a record the addressed participant can see.
func TestEffectAgentBusToolAskIsAddressedToOneParticipant(t *testing.T) {
	ctrl, _ := agentBusToolBuild(t, "boot-effect-agentbus-talk")
	busDir := filepath.Join(t.TempDir(), "agentbus", "default")
	ctrl.SetAgentBus(busDir, "alice")

	bound := &atomic.Pointer[control.Controller]{}
	bound.Store(ctrl)
	askTool := builtin.NewAgentBusTool(boardToolPort{ctrl: bound})
	out, err := askTool.Execute(context.Background(), json.RawMessage(
		`{"action":"ask","topic":"handover","to":"bob","text":"can you take the migration step?"}`))
	if err != nil {
		t.Fatalf("the tool refused the ask: %v", err)
	}
	if !strings.Contains(out, "bob") {
		t.Fatalf("the ask was not reported back as addressed: %q", out)
	}

	talkLog, err := agentbus.OpenTalkLog(busDir)
	if err != nil {
		t.Fatalf("open talk log: %v", err)
	}
	talk, _, err := talkLog.Read()
	if err != nil {
		t.Fatalf("read talk log: %v", err)
	}
	named, hidden := agentbus.Digest(talk, "handover", "bob", 0)
	if len(named) != 1 || named[0].To != "bob" || !strings.Contains(named[0].Text, "take the migration step") {
		t.Fatalf("bob is shown %+v, want the one ask addressed to him", named)
	}
	if hidden != 0 {
		t.Fatalf("hidden = %d, want the ask to name its addressee", hidden)
	}
	// A question that names nobody, or names everybody, is a cost bomb: someone else has
	// to see nothing of it.
	if other, _ := agentbus.Digest(talk, "handover", "carol", 0); len(other) != 0 {
		t.Fatalf("carol was shown %+v, want an ask to address only the participant it names", other)
	}
}
