package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
	"reasonix/internal/tool/builtin"
)

// AgentBusChildArg is one step a split creates, as the panel types it.
type AgentBusChildArg struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// AgentBusApplyArgs is the panel's action form. It mirrors the model's agent_bus
// arguments on purpose: a human and a model must not be able to disagree about what a
// verb means, so both go through the same tool.
type AgentBusApplyArgs struct {
	Action       string             `json:"action"`
	Node         string             `json:"node"`
	Title        string             `json:"title"`
	Reason       string             `json:"reason"`
	Outcome      string             `json:"outcome"`
	Ref          string             `json:"ref"`
	ReproducedBy string             `json:"reproducedBy"`
	Steps        int                `json:"steps"`
	Children     []AgentBusChildArg `json:"children"`
	Dep          *AgentBusChildArg  `json:"dep"`
}

// AgentBusApply runs one board action for the human driving the panel and returns the
// board's own answer: either what it recorded, or why it refused and what to change.
func (a *App) AgentBusApply(args AgentBusApplyArgs) (string, error) {
	ctrl := a.activeCtrl()
	if ctrl == nil {
		return "", fmt.Errorf("desktop: no active session")
	}
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return "", fmt.Errorf("desktop: this session cannot join a board")
	}
	raw, err := json.Marshal(agentBusToolPayload(args))
	if err != nil {
		return "", fmt.Errorf("desktop: encode the board action: %w", err)
	}
	return builtin.NewAgentBusTool(agentBusPanelPort{bus: bus}).Execute(context.Background(), raw)
}

// agentBusToolPayload reshapes the panel's flat form into the tool's arguments. The
// panel collects one evidence ref where the tool takes a list: one human-typed ref is
// one piece of evidence, and asking for a list in a single-line form would be theatre.
func agentBusToolPayload(args AgentBusApplyArgs) map[string]any {
	payload := map[string]any{"action": args.Action, "node": args.Node}
	if args.Title != "" {
		payload["title"] = args.Title
	}
	if args.Reason != "" {
		payload["reason"] = args.Reason
	}
	if args.Outcome != "" {
		payload["outcome"] = args.Outcome
	}
	if args.ReproducedBy != "" {
		payload["reproducedBy"] = args.ReproducedBy
	}
	if args.Ref != "" {
		payload["evidence"] = []map[string]string{{"ref": args.Ref}}
	}
	if args.Steps > 0 {
		payload["steps"] = args.Steps
	}
	if args.Dep != nil && args.Dep.ID != "" {
		dep := map[string]string{"id": args.Dep.ID}
		if args.Dep.Title != "" {
			dep["title"] = args.Dep.Title
		}
		payload["dep"] = dep
	}
	if len(args.Children) > 0 {
		children := make([]map[string]string, 0, len(args.Children))
		for _, child := range args.Children {
			entry := map[string]string{"id": child.ID}
			if child.Title != "" {
				entry["title"] = child.Title
			}
			children = append(children, entry)
		}
		payload["children"] = children
	}
	return payload
}

// agentBusPanelPort hands the tool the enrolled session's board. The panel reads the
// board through its own commands, but the tool is called with the same port shape the
// model's copy gets at boot, so a refusal is phrased identically in both.
type agentBusPanelPort struct {
	bus control.AgentBusControl
}

func (p agentBusPanelPort) BoardIdentity() (string, string, error) {
	return p.bus.AgentBusParticipant(), p.bus.AgentBusDir(), nil
}

func (p agentBusPanelPort) BoardView(now time.Time) (agentbus.View, error) {
	view, ok := p.bus.AgentBusView(now)
	if !ok {
		return agentbus.View{}, fmt.Errorf("desktop: the board could not be read")
	}
	return view, nil
}

func (p agentBusPanelPort) ApplyBoardOp(ctx context.Context, op board.Op) (board.Receipt, error) {
	return p.bus.ApplyAgentBusOp(ctx, op)
}

// The panel reads the roster and the pool through the same control calls the model's tool gets,
// so a person and a model cannot see different boards (F48/F49/F53, 2026-10-05).
func (p agentBusPanelPort) BoardParticipants() ([]agentbus.ParticipantRef, error) {
	return p.bus.AgentBusParticipants()
}

func (p agentBusPanelPort) BoardPool() ([]agentbus.PoolEntry, error) {
	return p.bus.AgentBusPool(time.Now().UTC())
}

func (p agentBusPanelPort) AskBoard(ctx context.Context, topic, to, text string) (string, error) {
	return p.bus.AgentBusAsk(ctx, topic, to, text)
}

func (p agentBusPanelPort) AnswerBoard(ctx context.Context, correlation, topic, to, text string) (uint64, error) {
	return p.bus.AgentBusAnswer(ctx, correlation, topic, to, text)
}

// The panel drives deliberations through the same three calls the model's tool uses, so a
// person and a model cannot disagree about what settling a refutation means (T11-5).
func (p agentBusPanelPort) OpenHearing(ctx context.Context, node string, required []string) (agentbus.HearingRecord, error) {
	return p.bus.OpenAgentBusHearing(ctx, node, required)
}

func (p agentBusPanelPort) AnswerHearing(ctx context.Context, node, text string, evidence []board.Evidence) (agentbus.HearingRecord, error) {
	return p.bus.AnswerAgentBusHearing(ctx, node, text, evidence)
}

func (p agentBusPanelPort) SettleHearing(ctx context.Context, node string) (agentbus.HearingRecord, error) {
	return p.bus.SettleAgentBusHearing(ctx, node)
}
