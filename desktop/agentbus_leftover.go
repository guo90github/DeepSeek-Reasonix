package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
)

// boardParticipantsOffline reports, per subtree root, the participants that subtree belongs to:
// owners, whoever the board addressed a step to, whoever asked for it, and whoever asserted on it.
func boardParticipantsOffline(state *board.State) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if state == nil {
		return out
	}
	for id, n := range state.Nodes {
		if n == nil {
			continue
		}
		root := agentbus.SubtreeRoot(state, id)
		set := out[root]
		if set == nil {
			set = map[string]bool{}
			out[root] = set
		}
		for _, actor := range []string{n.Owner, n.Assignee} {
			if actor = strings.TrimSpace(actor); actor != "" {
				set[actor] = true
			}
		}
		for _, requester := range n.Requesters {
			if requester = strings.TrimSpace(requester); requester != "" {
				set[requester] = true
			}
		}
		for _, assertion := range n.Asserts {
			if actor := strings.TrimSpace(assertion.Actor); actor != "" {
				set[actor] = true
			}
		}
	}
	return out
}

// agentBusCanReach reports whether this host has a route to a participant: a session here speaks as
// it, or the board's address book still owns it (an announcement past its TTL is gone).
func (a *App) agentBusCanReach(boardDir, participant string) bool {
	participant = strings.TrimSpace(participant)
	if participant == "" {
		return false
	}
	a.mu.RLock()
	tabs := make([]*WorkspaceTab, 0, len(a.tabs))
	for _, tab := range a.tabs {
		tabs = append(tabs, tab)
	}
	a.mu.RUnlock()
	for _, tab := range tabs {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		if bus, ok := tab.Ctrl.(control.AgentBusControl); ok && bus.AgentBusParticipant() == participant {
			return true
		}
	}
	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		return false
	}
	_, owned, err := directory.Lookup(participant)
	return err == nil && owned
}

// markLeftoverCards flags the cards whose participants are all out of reach from this host. A board
// outlives the sessions that wrote to it, so those rows are history rather than a call to act — and
// the panel says which is which instead of leaving the reader to guess (2026-10-05). It is a read:
// nothing on the board changes, and a card nobody can act on stays visible under "leftover".
func (a *App) markLeftoverCards(boardDir string, cards []AgentBusCardView) {
	if strings.TrimSpace(boardDir) == "" || len(cards) == 0 {
		return
	}
	brd, err := board.Open(boardDir)
	if err != nil {
		return
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		return
	}
	involved := boardParticipantsOffline(state)
	known := map[string]bool{}
	for i := range cards {
		set := involved[cards[i].Subtree]
		if len(set) == 0 {
			continue
		}
		reachable := false
		for participant := range set {
			ok, seen := known[participant]
			if !seen {
				ok = a.agentBusCanReach(boardDir, participant)
				known[participant] = ok
			}
			if ok {
				reachable = true
				break
			}
		}
		cards[i].Leftover = !reachable
	}
}

// AgentBusRetireArgs names the subtree the human wants cleared.
type AgentBusRetireArgs struct {
	Subtree string `json:"subtree"`
}

// AgentBusRetireSubtree drops the steps of one leftover subtree: each one first asks the board to
// abandon it (evidence is the reachability fact this host just read) and then records the decision,
// because the kernel keeps those two apart on purpose. The human is a participant, so this goes
// through the same op vocabulary the model uses — there is no side door into the log.
func (a *App) AgentBusRetireSubtree(args AgentBusRetireArgs) (string, error) {
	subtree := strings.TrimSpace(args.Subtree)
	if subtree == "" {
		return "", fmt.Errorf("desktop: no subtree named")
	}
	ctrl := a.activeCtrl()
	if ctrl == nil {
		return "", fmt.Errorf("desktop: no active session")
	}
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return "", fmt.Errorf("desktop: this session does not join a board")
	}
	boardDir := strings.TrimSpace(bus.AgentBusDir())
	if boardDir == "" {
		return "", fmt.Errorf("desktop: this session has no board directory")
	}
	brd, err := board.Open(boardDir)
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		return "", err
	}
	actor := bus.AgentBusParticipant()
	targets := make([]string, 0, len(state.Nodes))
	for id, n := range state.Nodes {
		if n == nil || agentbus.SubtreeRoot(state, id) != subtree {
			continue
		}
		switch n.State {
		case board.StateDone, board.StateAbandoned:
			continue
		}
		targets = append(targets, id)
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		return fmt.Sprintf("「%s」没有可退的步骤", subtree), nil
	}
	evidence := []board.Evidence{{
		Kind: "manual",
		Ref:  "desktop: nobody who asked for this subtree is reachable from this host (no tab here, no live announced address)",
	}}
	retired := 0
	for _, node := range targets {
		if _, err := bus.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbAbandon, Node: node, Actor: actor,
			Reason:   "leftover work: nobody who asked for it can be reached from this host",
			Evidence: evidence,
		}); err != nil {
			return fmt.Sprintf("退了 %d 步，「%s」上还有没退成的：%v", retired, node, err), nil
		}
		if _, err := bus.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbDecide, Node: node, Actor: actor, Outcome: board.OutcomeAbandoned,
		}); err != nil {
			return fmt.Sprintf("退了 %d 步，「%s」上还有没退成的：%v", retired, node, err), nil
		}
		retired++
	}
	return fmt.Sprintf("已退掉「%s」的 %d 步", subtree, retired), nil
}
