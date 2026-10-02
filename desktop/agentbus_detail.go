package main

import (
	"fmt"

	"reasonix/internal/control"
)

// AgentBusAuthorizationView is one approval as the panel shows it: who, why, and which
// record it came from. The folded board cannot answer this — the op log can (§13.8).
type AgentBusAuthorizationView struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Seq    uint64 `json:"seq"`
}

// AgentBusRefutationView is one challenge on a step, with the reason it was made.
type AgentBusRefutationView struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// AgentBusNodeDetailView is one step opened from the collaboration panel: what it is, what
// it waits for, what is in doubt and who authorized it. Read-only by design: the panel
// decides nothing about a step, it only shows what the record says.
type AgentBusNodeDetailView struct {
	Node           string                      `json:"node"`
	Title          string                      `json:"title"`
	State          string                      `json:"state"`
	Owner          string                      `json:"owner"`
	Ready          bool                        `json:"ready"`
	Deps           []string                    `json:"deps"`
	NoProgress     int                         `json:"noProgress"`
	Refutations    []AgentBusRefutationView    `json:"refutations"`
	Authorizations []AgentBusAuthorizationView `json:"authorizations"`
	Deliberating   bool                        `json:"deliberating"`
	Verdict        string                      `json:"verdict"`
}

// AgentBusNodeDetail reads one step of the active session's board. A session that never
// joined a board, or a node the board does not hold, reports that plainly instead of
// drawing an empty step.
func (a *App) AgentBusNodeDetail(node string) (AgentBusNodeDetailView, error) {
	ctrl := a.activeCtrl()
	if ctrl == nil {
		return AgentBusNodeDetailView{}, fmt.Errorf("desktop: no active session")
	}
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return AgentBusNodeDetailView{}, fmt.Errorf("desktop: this session does not join a board")
	}
	detail, ok := bus.AgentBusNodeDetail(node)
	if !ok {
		return AgentBusNodeDetailView{}, fmt.Errorf("desktop: the board does not hold %q", node)
	}
	view := AgentBusNodeDetailView{
		Node: detail.Node, Title: detail.Title, State: string(detail.State), Owner: detail.Owner,
		Ready: detail.Ready, Deps: detail.Deps, NoProgress: detail.NoProgress,
		Deliberating: detail.Deliberating, Verdict: detail.Verdict,
	}
	for _, refutation := range detail.Refutations {
		view.Refutations = append(view.Refutations, AgentBusRefutationView{
			Actor: refutation.Actor, Reason: refutation.Reason,
		})
	}
	for _, authorization := range detail.Authorizations {
		view.Authorizations = append(view.Authorizations, AgentBusAuthorizationView{
			Actor: authorization.Actor, Reason: authorization.Reason, Seq: authorization.Seq,
		})
	}
	return view, nil
}
