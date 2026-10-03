package serve

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
)

// installAgentBusWaker gives a hosted session the host's routing for ready-work wakes.
// Without a waker the kernel wakes nobody — which is why a headless participant could only
// ever be pushed by a write: the desktop installed one and serve never did (§13.2).
func (s *Server) installAgentBusWaker(ctrl *control.Controller) {
	ctrl.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		return s.deliverAgentBusWake(target)
	})
}

// deliverAgentBusWake puts a wake in the inbox of the session here that speaks as the
// participant, which starts a turn on an idle one — that is the whole point of a wake.
//
// A participant no session here speaks as is a refusal, never a drop: the sender has to see
// that nobody heard, and the host that really owns it delivers for its own participants.
func (s *Server) deliverAgentBusWake(target agentbus.WakeTarget) error {
	ctrl, err := s.controllerForParticipant(target.Participant)
	if err != nil {
		return err
	}
	text := control.AgentBusWakePrompt(target)
	_, err = ctrl.TryEnqueueFollowup(control.InboxRequest{
		Submit:      text,
		Raw:         text,
		Display:     control.AgentBusWakeLine(target),
		Source:      control.AgentBusWakeSource,
		Idempotency: target.Key,
	})
	return err
}

// controllerForParticipant resolves a participant to the one session that speaks as it. Two
// sessions claiming one participant is reported rather than guessed: waking the wrong
// session is the failure this routing exists to prevent.
func (s *Server) controllerForParticipant(participant string) (*control.Controller, error) {
	participant = strings.TrimSpace(participant)
	if participant == "" {
		return nil, fmt.Errorf("serve: a wake naming no participant reaches nobody")
	}
	var found *control.Controller
	for _, ctrl := range s.taggedControllers() {
		if ctrl.AgentBusParticipant() != participant {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("serve: two sessions here speak as agentbus participant %q", participant)
		}
		found = ctrl
	}
	if found == nil {
		return nil, fmt.Errorf("serve: no session here speaks as agentbus participant %q", participant)
	}
	return found, nil
}
