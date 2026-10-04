package main

import (
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

// The panel has to answer "who is here with me" without the reader counting the departed names a
// board still carries: the roster is the sessions this host speaks as, and exactly one of them is
// the session doing the reading (2026-10-05).
func TestTheRosterNamesTheSessionsOnThisHost(t *testing.T) {
	app, ctrl, _ := agentBusEnrolApp(t)
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("join: %v", err)
	}
	peer := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	t.Cleanup(peer.Close)
	peer.SetAgentBus(ctrl.AgentBusDir(), "peer")
	app.mu.Lock()
	app.tabs["t2"] = &WorkspaceTab{ID: "t2", Ctrl: peer, TopicTitle: "另一个会话"}
	app.mu.Unlock()

	view, err := app.AgentBusBriefing()
	if err != nil {
		t.Fatalf("briefing: %v", err)
	}
	if len(view.Members) != 2 {
		t.Fatalf("members = %+v, want the two sessions on this host", view.Members)
	}
	self, named := 0, false
	for _, member := range view.Members {
		if member.Self {
			self++
			if member.Participant != view.Participant {
				t.Fatalf("the session marked self is %q, want the reader %q", member.Participant, view.Participant)
			}
		}
		if member.Label == "另一个会话" {
			named = true
		}
	}
	if self != 1 {
		t.Fatalf("members = %+v, want exactly one marked self", view.Members)
	}
	if !named {
		t.Fatalf("members = %+v, want the other session under the name it shows", view.Members)
	}
}
