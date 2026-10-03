package control

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/config"
)

// SetAgentBusHearingLimits sets the deliberation bounds this session's hearings
// obey. Zero values leave a bound off, which is the default.
func (c *Controller) SetAgentBusHearingLimits(limits agentbus.HearingLimits) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.hearingLimits = limits
	}
}

// AgentBusHearingLimits maps the operator's deliberation knobs onto the kernel's bounds. Zero
// stays zero: a host nobody configured deliberates without a clock, which is also why the side
// that owes an answer is never woken (hearing.go: silence needs a round window).
func AgentBusHearingLimits(cfg config.AgentBusConfig) agentbus.HearingLimits {
	return agentbus.HearingLimits{
		MaxRounds:       cfg.HearingMaxRounds,
		RoundTTL:        time.Duration(cfg.HearingRoundTTLMinutes) * time.Minute,
		Cooldown:        time.Duration(cfg.HearingCooldownMinutes) * time.Minute,
		EscalationQuota: cfg.HearingEscalationQuota,
	}
}

// OpenAgentBusHearing opens a deliberation on a contested node. An empty required
// list means "whoever must answer": the node's owner and everyone who refuted it.
func (c *Controller) OpenAgentBusHearing(ctx context.Context, node string, required []string) (agentbus.HearingRecord, error) {
	bus, _, boardState, err := c.agentBusHearingSnapshot(ctx)
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	if len(required) == 0 {
		required = agentbus.HearingRequired(boardState, node)
	}
	log, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	record, err := log.Append(ctx, agentbus.HearingRecord{
		Node:     strings.TrimSpace(node),
		Kind:     agentbus.HearingOpen,
		Actor:    bus.participantID(c),
		Required: required,
	}, bus.limitsForHearing())
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	// The writer that changes who owes an answer wakes them — no host tick required. The
	// key is derived from the work set, so waking again over an unchanged deliberation
	// wakes nobody: that missing wake is what left this chain unrun in production (G1).
	c.WakeAgentBus(ctx)
	return record, nil
}

// AnswerAgentBusHearing records this participant's answer in the current round.
// Evidence is what the answer is worth: an answer that brings nothing checkable
// weighs nothing.
func (c *Controller) AnswerAgentBusHearing(ctx context.Context, node, text string, evidence []board.Evidence) (agentbus.HearingRecord, error) {
	bus, _, _, err := c.agentBusHearingSnapshot(ctx)
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	log, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	record, err := log.Append(ctx, agentbus.HearingRecord{
		Node:     strings.TrimSpace(node),
		Kind:     agentbus.HearingAnswer,
		Actor:    bus.participantID(c),
		Text:     text,
		Evidence: evidence,
	}, bus.limitsForHearing())
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	c.WakeAgentBus(ctx)
	return record, nil
}

// SettleAgentBusHearing weighs a deliberation and records the verdict. A refuted
// assertion blocks the node; a standing one is left to its producer's own
// decide(done), because a hearing may not manufacture the evidence that gate
// demands (T6-5).
func (c *Controller) SettleAgentBusHearing(ctx context.Context, node string) (agentbus.HearingRecord, error) {
	bus, hearings, boardState, err := c.agentBusHearingSnapshot(ctx)
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	node = strings.TrimSpace(node)
	if hearings.Hearings[node] == nil {
		return agentbus.HearingRecord{}, fmt.Errorf("control: no hearing on %q", node)
	}
	log, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	record, err := log.Weigh(ctx, node, claimEvidence(boardState, node), answeredEvidence(hearings.Hearings[node]), bus.participantID(c), bus.limitsForHearing())
	if err != nil {
		return agentbus.HearingRecord{}, err
	}
	if record.Verdict == agentbus.VerdictRefuted {
		if _, err := c.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbDecide, Node: node, Actor: bus.participantID(c), Outcome: board.OutcomeBlocked,
		}); err != nil {
			return record, fmt.Errorf("control: block the refuted node %q: %w", node, err)
		}
	}
	c.WakeAgentBus(ctx)
	return record, nil
}

// AgentBusHearings projects the deliberations for the human side, sorted by node
// so two reads agree.
func (c *Controller) AgentBusHearings(ctx context.Context) ([]agentbus.Hearing, bool) {
	_, hearings, _, err := c.agentBusHearingSnapshot(ctx)
	if err != nil {
		return nil, false
	}
	nodes := make([]string, 0, len(hearings.Hearings))
	for node := range hearings.Hearings {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	out := make([]agentbus.Hearing, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, *hearings.Hearings[node])
	}
	return out, true
}

func (c *Controller) agentBusHearingSnapshot(ctx context.Context) (*agentBusState, *agentbus.HearingState, *board.State, error) {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return nil, nil, nil, err
	}
	brd, err := board.Open(bus.dir)
	if err != nil {
		return nil, nil, nil, err
	}
	boardState, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		return nil, nil, nil, err
	}
	log, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return nil, nil, nil, err
	}
	hearings, _, err := log.Read()
	if err != nil {
		return nil, nil, nil, err
	}
	return bus, hearings, boardState, nil
}

// claimEvidence is what the node's producer brought: the evidence of every
// assertion made by whoever holds the node.
func claimEvidence(st *board.State, node string) []board.Evidence {
	n, ok := st.Nodes[node]
	if !ok {
		return nil
	}
	out := []board.Evidence{}
	for _, assertion := range n.Asserts {
		if n.Owner != "" && assertion.Actor != n.Owner {
			continue
		}
		out = append(out, assertion.Evidence...)
	}
	return out
}

// answeredEvidence is what the deliberation's answers brought, from every side.
func answeredEvidence(h *agentbus.Hearing) []board.Evidence {
	if h == nil {
		return nil
	}
	out := []board.Evidence{}
	for _, rec := range h.Records {
		if rec.Kind == agentbus.HearingAnswer {
			out = append(out, rec.Evidence...)
		}
	}
	return out
}

func (b *agentBusState) limitsForHearing() agentbus.HearingLimits {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hearingLimits
}
