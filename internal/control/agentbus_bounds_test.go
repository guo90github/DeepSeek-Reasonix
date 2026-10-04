package control

import (
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/config"
)

// The bounds an operator sets belong to the session: living only in the state SetAgentBus
// rebuilds, any re-enrolment silently dropped them (F5b, 2026-10-05).
func TestOperatorBoundsSurviveReEnrolment(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "bob")

	talk := agentbus.TalkLimits{SilenceWindow: 3 * time.Minute, MaxRounds: 2}
	hearing := agentbus.HearingLimits{MaxRounds: 4, EscalationQuota: 1}
	rate := board.Limits{NodeRatePerMinute: 7}
	c.SetAgentBusTalkLimits(talk)
	c.SetAgentBusHearingLimits(hearing)
	c.SetAgentBusNodeRate(rate)

	c.SetAgentBus(dir, "bob")

	c.mu.Lock()
	defer c.mu.Unlock()
	if got := c.agentBus.limits; got != talk {
		t.Fatalf("talk limits after re-enrolment = %+v, want %+v", got, talk)
	}
	if got := c.agentBus.hearingLimits; got != hearing {
		t.Fatalf("hearing limits after re-enrolment = %+v, want %+v", got, hearing)
	}
	if got := c.agentBus.nodeRate; got != rate {
		t.Fatalf("node rate after re-enrolment = %+v, want %+v", got, rate)
	}
}

// The operator's talk knobs reach the kernel's bounds, and an unconfigured host keeps every
// bound off: the kernel invents no ceilings on the operator's behalf (2026-10-05).
func TestConfiguredTalkBoundsMapOntoTheKernel(t *testing.T) {
	limits := AgentBusTalkLimits(config.AgentBusConfig{
		Talk: config.AgentBusTalkConfig{
			SilenceMinutes: 2, MaxRounds: 1, RatePerMinute: 3, AskTTLMinutes: 5, MaxHop: 4,
		},
	})
	want := agentbus.TalkLimits{
		MaxRounds: 1, SilenceWindow: 2 * time.Minute,
		RateWindow: time.Minute, RateMax: 3,
		AskTTL: 5 * time.Minute, MaxHop: 4,
	}
	if limits != want {
		t.Fatalf("talk limits = %+v, want %+v", limits, want)
	}
	if empty := AgentBusTalkLimits(config.AgentBusConfig{}); empty != (agentbus.TalkLimits{}) {
		t.Fatalf("an unconfigured host got bounds %+v, want every bound off", empty)
	}
}
