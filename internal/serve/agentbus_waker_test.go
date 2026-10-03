package serve

import (
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

func agentBusSession(t *testing.T, boardDir, participant string) *control.Controller {
	t.Helper()
	// A real session path: an inbox item belongs to a session, and serve's controllers
	// always have one.
	sessionDir := t.TempDir()
	ctrl := control.New(control.Options{
		SessionDir:  sessionDir,
		SessionPath: filepath.Join(sessionDir, "session.jsonl"),
		Sink:        event.Discard,
	})
	ctrl.SetAgentBus(boardDir, participant)
	// The inbox keeps files open under the session dir; on Windows that makes the temporary
	// directory unremovable unless the session is closed first.
	t.Cleanup(ctrl.Close)
	return ctrl
}

// A hosted session is the only host its participant has, so a ready-work wake has to reach
// that session's inbox — otherwise a headless participant moves only when somebody else writes.
func TestHostedSessionReceivesAWakeForItsOwnParticipant(t *testing.T) {
	bob := agentBusSession(t, t.TempDir(), "bob")
	s := &Server{tags: map[*control.Controller]*sessionTagSink{bob: nil}}
	s.installAgentBusWaker(bob)

	target := agentbus.WakeTarget{Participant: "bob", Key: "agentbus-wake:bob:deadbeef", Ready: []string{"schema"}}
	if err := s.deliverAgentBusWake(target); err != nil {
		t.Fatalf("deliver wake: %v", err)
	}
	items := bob.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox has %d items, want the wake queued", len(items))
	}
	if items[0].Source != control.AgentBusWakeSource || items[0].Idempotency != target.Key {
		t.Fatalf("queued item = %+v, want the wake keyed by its own wake", items[0])
	}
}

// A participant no session here speaks as is a refusal, never a drop: the sender has to see
// that nobody heard, and the host that really owns it delivers for its own participants.
func TestWakeForAParticipantNoSessionSpeaksAsIsRefused(t *testing.T) {
	s := &Server{tags: map[*control.Controller]*sessionTagSink{}}
	err := s.deliverAgentBusWake(agentbus.WakeTarget{Participant: "ghost", Key: "k"})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("err = %v, want a refusal naming the participant", err)
	}
}

// Two sessions claiming one participant is reported rather than guessed: waking the wrong
// session is the failure this routing exists to prevent.
func TestTwoSessionsClaimingOneParticipantAreRefused(t *testing.T) {
	dir := t.TempDir()
	first := agentBusSession(t, dir, "bob")
	second := agentBusSession(t, dir, "bob")
	s := &Server{tags: map[*control.Controller]*sessionTagSink{first: nil, second: nil}}
	if _, err := s.controllerForParticipant("bob"); err == nil || !strings.Contains(err.Error(), "two sessions") {
		t.Fatalf("err = %v, want the ambiguity reported", err)
	}
}
