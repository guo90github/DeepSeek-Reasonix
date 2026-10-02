package main

import (
	"fmt"
	"time"

	"reasonix/internal/control"
)

// AgentBusCardView is one subtree card the collaboration panel draws: counts plus
// how many things are wrong with it.
type AgentBusCardView struct {
	Subtree  string `json:"subtree"`
	Nodes    int    `json:"nodes"`
	AtWork   int    `json:"atWork"`
	Parked   int    `json:"parked"`
	Done     int    `json:"done"`
	Worst    string `json:"worst"`
	Signals  int    `json:"signals"`
	Orphans  int    `json:"orphans"`
	Stalled  int    `json:"stalled"`
	Disputed int    `json:"disputed"`
}

// AgentBusSignalView is one drill-in row behind a card. Node is the address the
// frontend opens when the row is clicked.
type AgentBusSignalView struct {
	Kind    string `json:"kind"`
	Subtree string `json:"subtree"`
	Node    string `json:"node"`
	Detail  string `json:"detail"`
}

// AgentBusBriefingView is the collaboration surface's first screen: only the
// subtrees that need attention, with honest counts of what did not fit.
type AgentBusBriefingView struct {
	Participant     string               `json:"participant"`
	Cards           []AgentBusCardView   `json:"cards"`
	Signals         []AgentBusSignalView `json:"signals"`
	Hidden          int                  `json:"hidden"`
	HiddenCards     int                  `json:"hiddenCards"`
	HealthySubtrees int                  `json:"healthySubtrees"`
}

// AgentBusBriefing is the panel's read: it folds the active session's board, queue
// and deliberations. A session that never joined a board reports that plainly
// instead of showing an empty screen.
func (a *App) AgentBusBriefing() (AgentBusBriefingView, error) {
	ctrl := a.activeCtrl()
	if ctrl == nil {
		return AgentBusBriefingView{}, fmt.Errorf("desktop: no active session")
	}
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return AgentBusBriefingView{}, fmt.Errorf("desktop: this session does not join a board")
	}
	briefing, ok := bus.AgentBusBriefing(time.Now().UTC())
	if !ok {
		return AgentBusBriefingView{}, fmt.Errorf("desktop: this session does not join a board")
	}
	view := AgentBusBriefingView{
		Participant:     bus.AgentBusParticipant(),
		Hidden:          briefing.Hidden,
		HiddenCards:     briefing.HiddenCards,
		HealthySubtrees: briefing.HealthySubtrees,
	}
	for _, card := range briefing.Cards {
		view.Cards = append(view.Cards, AgentBusCardView{
			Subtree: card.Subtree, Nodes: card.Nodes, AtWork: card.AtWork, Parked: card.Parked,
			Done: card.Done, Worst: string(card.Worst), Signals: card.Signals,
			Orphans: card.Orphans, Stalled: card.Stalled, Disputed: card.Disputed,
		})
	}
	for _, signal := range briefing.Signals {
		view.Signals = append(view.Signals, AgentBusSignalView{
			Kind: string(signal.Kind), Subtree: signal.Subtree, Node: signal.Node, Detail: signal.Detail,
		})
	}
	return view, nil
}
