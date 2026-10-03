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
// A participant no session here speaks as is handed to the address book instead: without that
// fallback a headless host's wakes died locally, so only a desktop could ever push one across
// processes. Nothing anywhere is still a refusal, never a drop — a sender has to see that
// nobody heard it.
func (s *Server) deliverAgentBusWake(target agentbus.WakeTarget) error {
	ctrl, err := s.localControllerForParticipant(target.Participant)
	if err != nil {
		return err
	}
	if ctrl != nil {
		return enqueueAgentBusWake(ctrl, target)
	}
	return s.deliverAgentBusWakeRemotely(target)
}

// enqueueAgentBusWake hands the wake to the session as a durable follow-up keyed by the wake
// itself, so a repeat collapses and an idle session actually gets a turn out of it.
func enqueueAgentBusWake(ctrl *control.Controller, target agentbus.WakeTarget) error {
	text := control.AgentBusWakePrompt(target)
	_, err := ctrl.TryEnqueueFollowup(control.InboxRequest{
		Submit:      text,
		Raw:         text,
		Display:     control.AgentBusWakeLine(target),
		Source:      control.AgentBusWakeSource,
		Idempotency: target.Key,
	})
	return err
}

// deliverAgentBusWakeRemotely asks the board's address book where this participant speaks from
// and posts the wake there with the token that host announced.
func (s *Server) deliverAgentBusWakeRemotely(target agentbus.WakeTarget) error {
	boardDir := strings.TrimSpace(s.buildOptions.AgentBusDir)
	if boardDir == "" {
		return fmt.Errorf("serve: no session here speaks as agentbus participant %q, and this host has no board to look up an address in", target.Participant)
	}
	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		return err
	}
	ref, ok, err := directory.Lookup(target.Participant)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("serve: no session here speaks as agentbus participant %q, and no address owns it", target.Participant)
	}
	token, err := agentbus.ReadAnnouncedToken(ref.TokenFile)
	if err != nil {
		return err
	}
	return agentbus.DeliverWake(context.Background(), nil, agentbus.WakeDelivery{
		BaseURL:       ref.Host,
		Token:         token,
		SessionHeader: agentbus.SessionPathHeader,
		SessionPath:   ref.SessionPath,
	}, agentbus.WakeMessage{
		Prompt:  control.AgentBusWakePrompt(target),
		Display: control.AgentBusWakeLine(target),
		Key:     target.Key,
	})
}

// localControllerForParticipant resolves a participant to the one session here that speaks as
// it, or nil when nobody here does. Two sessions claiming one participant is reported rather
// than guessed: waking the wrong session is the failure this routing exists to prevent.
func (s *Server) localControllerForParticipant(participant string) (*control.Controller, error) {
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
	return found, nil
}

// controllerForParticipant is localControllerForParticipant with "nobody here" reported as an
// error, for callers that read a participant's node detail rather than routing a wake.
func (s *Server) controllerForParticipant(participant string) (*control.Controller, error) {
	ctrl, err := s.localControllerForParticipant(participant)
	if err != nil {
		return nil, err
	}
	if ctrl == nil {
		return nil, fmt.Errorf("serve: no session here speaks as agentbus participant %q", strings.TrimSpace(participant))
	}
	return ctrl, nil
}
