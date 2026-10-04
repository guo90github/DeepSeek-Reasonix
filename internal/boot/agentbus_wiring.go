package boot

import (
	"log/slog"
	"sync"

	"reasonix/internal/agentbus"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

// enrolAgentBusController joins the built controller to the board it was asked to speak on,
// applies the operator's deliberation bounds, and tells the board where this host can be
// reached. It sits outside build() so one place owns the whole enrolment.
func enrolAgentBusController(ctrl *control.Controller, cfg *config.Config, opts Options) {
	if opts.AgentBusDir == "" {
		return
	}
	ctrl.SetAgentBus(opts.AgentBusDir, opts.AgentBusID)
	// A deliberation is only woken once its round window lapses, so a host with no window
	// never wakes the side that owes an answer (AGENT_BUS §11.5.7).
	limits := control.AgentBusHearingLimits(cfg.AgentBus)
	ctrl.SetAgentBusHearingLimits(limits)
	noticeAgentBusHearingWindow(limits)
	// §S5's fourth hard limit is the board's own rule — how often one node may move — so the
	// kernel holds it and the host only passes the operator's number along.
	ctrl.SetAgentBusNodeRate(control.AgentBusNodeRate(cfg.AgentBus))
	// Talk's five bounds are the operator's too, and until now nothing read them on this path:
	// the desktop needed its own wiring for the same reason the hearing bounds did (2026-10-05).
	talkLimits := control.AgentBusTalkLimits(cfg.AgentBus)
	ctrl.SetAgentBusTalkLimits(talkLimits)
	noticeAgentBusTalkWindow(talkLimits)
	// A host that serves its own sessions tells the board where they speak from, so another
	// host can wake them. The desktop passes nothing here (it serves no session endpoint yet)
	// and is then a wake sender only.
	if opts.AgentBusHost == "" {
		return
	}
	if err := ctrl.AgentBusAnnounce(opts.AgentBusHost, opts.AgentBusTokenFile); err != nil {
		slog.Warn("boot: agentbus announce", "err", err)
	}
}

// agentBusHearingWindowNotice fires once per process: no board can show that its
// deliberations have no clock, and every session this host builds is affected alike.
var agentBusHearingWindowNotice sync.Once

func noticeAgentBusHearingWindow(limits agentbus.HearingLimits) {
	if limits.RoundTTL > 0 {
		return
	}
	agentBusHearingWindowNotice.Do(func() {
		slog.Warn("boot: this host runs deliberations with no round window, so a hearing nobody answers is never counted, woken or closed; set [agentbus] hearing_round_ttl_minutes in reasonix.toml")
	})
}

// agentBusTalkWindowNotice fires once per process for the same reason as the hearing one: no
// board can show that its topics never close by silence, and every session here is alike.
var agentBusTalkWindowNotice sync.Once

func noticeAgentBusTalkWindow(limits agentbus.TalkLimits) {
	if limits.SilenceWindow > 0 {
		return
	}
	agentBusTalkWindowNotice.Do(func() {
		slog.Warn("boot: this host runs talk with no silence window, so no topic is ever closed by silence; set [agentbus.talk] silence_minutes in reasonix.toml")
	})
}
