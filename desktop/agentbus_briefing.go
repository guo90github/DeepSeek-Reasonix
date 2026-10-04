package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/provider"
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
	// Leftover marks a subtree whose participants this host cannot reach at all: a board outlives
	// the sessions that wrote to it, so those rows are history rather than a call to act. The
	// panel folds them away by default and can retire them on request (2026-10-05).
	Leftover bool `json:"leftover"`
}

// AgentBusSignalView is one drill-in row behind a card. Node is the address the
// frontend opens when the row is clicked.
type AgentBusSignalView struct {
	Kind    string `json:"kind"`
	Subtree string `json:"subtree"`
	Node    string `json:"node"`
	Detail  string `json:"detail"`
}

// AgentBusMemberView is one session taking part from this host: the panel's roster, so a reader can
// see who is actually here instead of counting the departed names a board still carries.
type AgentBusMemberView struct {
	Participant string `json:"participant"`
	Label       string `json:"label"`
	Self        bool   `json:"self"`
}

// AgentBusBriefingView is the collaboration surface's first screen: only the
// subtrees that need attention, with honest counts of what did not fit.
type AgentBusBriefingView struct {
	Participant     string               `json:"participant"`
	Members         []AgentBusMemberView `json:"members"`
	Cards           []AgentBusCardView   `json:"cards"`
	Signals         []AgentBusSignalView `json:"signals"`
	Hidden          int                  `json:"hidden"`
	HiddenCards     int                  `json:"hiddenCards"`
	HealthySubtrees int                  `json:"healthySubtrees"`
}

// agentBusMembers lists the sessions this host speaks as. It is host-scoped on purpose: who exists
// elsewhere is the board's and the address book's question, and this line answers the one a reader
// asks first — "who is here with me" (2026-10-05).
func (a *App) agentBusMembers(self string) []AgentBusMemberView {
	a.mu.RLock()
	tabs := make([]*WorkspaceTab, 0, len(a.tabs))
	for _, tab := range a.tabs {
		tabs = append(tabs, tab)
	}
	a.mu.RUnlock()
	out := make([]AgentBusMemberView, 0, len(tabs))
	seen := map[string]bool{}
	for _, tab := range tabs {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		bus, ok := tab.Ctrl.(control.AgentBusControl)
		if !ok {
			continue
		}
		participant := strings.TrimSpace(bus.AgentBusParticipant())
		if participant == "" || seen[participant] {
			continue
		}
		seen[participant] = true
		label := strings.TrimSpace(tab.TopicTitle)
		if label == "" {
			label = strings.TrimSpace(tab.Label)
		}
		out = append(out, AgentBusMemberView{Participant: participant, Label: label, Self: participant == self})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Self != out[j].Self {
			return out[i].Self
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Participant < out[j].Participant
	})
	return out
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
		Participant: bus.AgentBusParticipant(),
		Members:     a.agentBusMembers(bus.AgentBusParticipant()),
		// Empty, not nil: a Go nil slice marshals to null, and the panel iterates
		// these lists, so a board nobody has written to must still send arrays.
		Cards:           make([]AgentBusCardView, 0, len(briefing.Cards)),
		Signals:         make([]AgentBusSignalView, 0, len(briefing.Signals)),
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
	a.markLeftoverCards(bus.AgentBusDir(), view.Cards)
	for _, signal := range briefing.Signals {
		view.Signals = append(view.Signals, AgentBusSignalView{
			Kind: string(signal.Kind), Subtree: signal.Subtree, Node: signal.Node, Detail: signal.Detail,
		})
	}
	if signal, refused := budgetRefusalSignal(control.AgentBusBudgetRefusals()); refused {
		view.Signals = append(view.Signals, signal)
	}
	if signal, failed := wakeFailureSignal(control.AgentBusWakeFailures()); failed {
		view.Signals = append(view.Signals, signal)
	}
	// The unreachable row has to agree with what the panel calls attention: name a participant only
	// when the board is showing work their absence blocks. Retiring that work empties the row, and
	// unowned startable steps never put anyone in it (anyone here may take those).
	interesting := make([]string, 0, len(view.Cards)+len(view.Signals))
	for _, card := range view.Cards {
		interesting = append(interesting, card.Subtree)
	}
	for _, signal := range view.Signals {
		if signal.Node != "" {
			interesting = append(interesting, signal.Node)
		}
	}
	if signal, unreachable := wakeUnreachableSignal(a.agentBusUnreachableAmong(bus.AgentBusDir(), interesting)); unreachable {
		view.Signals = append(view.Signals, signal)
	}
	if signal, throttled := nodeRateSignal(control.AgentBusNodeRateRefusals()); throttled {
		view.Signals = append(view.Signals, signal)
	}
	if signal, throttled := rateLimitSignal(provider.RateLimitRetriesByProvider(), provider.RateLimitRetries()); throttled {
		view.Signals = append(view.Signals, signal)
	}
	return view, nil
}

// rateLimitSignal turns the 429s this host rode out into one row. The process total says it is
// happening at all; the per-provider counts say which lane, which is what a multi-provider
// operator has to act on. A 429 the provider did not name is in the total only — never guessed.
func rateLimitSignal(byProvider map[string]int64, total int64) (AgentBusSignalView, bool) {
	if total == 0 {
		return AgentBusSignalView{}, false
	}
	detail := fmt.Sprintf("本进程扛过 %d 次限流（429）", total)
	if len(byProvider) > 0 {
		lanes := make([]string, 0, len(byProvider))
		for id, count := range byProvider {
			lanes = append(lanes, fmt.Sprintf("%s %d", id, count))
		}
		sort.Strings(lanes)
		detail += "：" + strings.Join(lanes, "、")
	}
	return AgentBusSignalView{Kind: "rate_limited", Detail: detail}, true
}

// budgetRefusalSignal turns the host's refusal counts into one row a person can act on: which
// ceiling turned work down and how often. The board's own projection has no refusals in it —
// the account belongs to the machine — so the host adds this row where a reader already looks
// for what needs attention. Nothing refused means no row.
func budgetRefusalSignal(counts control.BudgetRefusalCounts) (AgentBusSignalView, bool) {
	if counts.Total() == 0 {
		return AgentBusSignalView{}, false
	}
	levels := []struct {
		name  string
		count int64
	}{
		{"board", counts.Board}, {"subtree", counts.Subtree}, {"node", counts.Node},
		{"turn", counts.Turn}, {"slots", counts.Slots},
	}
	named := make([]string, 0, len(levels))
	for _, level := range levels {
		if level.count > 0 {
			named = append(named, fmt.Sprintf("%s %d", level.name, level.count))
		}
	}
	return AgentBusSignalView{
		Kind:   "budget",
		Detail: fmt.Sprintf("有 %d 次认领被上限拒收：%s", counts.Total(), strings.Join(named, "、")),
	}, true
}

// wakeFailureSignal turns wakes the host could not hand over into one row a person can act on.
// A wake that reaches nobody leaves the board standing still, and without this row the sender’s
// log is its only witness — the same invisible brake the refusal row exists for (G3/T12-3).
func wakeFailureSignal(failures control.WakeFailures) (AgentBusSignalView, bool) {
	if failures.Count == 0 {
		return AgentBusSignalView{}, false
	}
	detail := fmt.Sprintf("本进程有 %d 次唤醒没能投给参与者", failures.Count)
	if failures.Last != "" {
		detail += "：" + failures.Last
	}
	return AgentBusSignalView{Kind: "wake_undelivered", Detail: detail}, true
}

// nodeRateSignal turns moves the board's own rate ceiling turned down into one row. A refused
// move lands nothing, so nothing else on the board shows it: without this row the brake is
// invisible (G3).
func nodeRateSignal(refusals control.NodeRateRefusals) (AgentBusSignalView, bool) {
	if refusals.Count == 0 {
		return AgentBusSignalView{}, false
	}
	detail := fmt.Sprintf("本进程有 %d 次节点迁移被速率上限拒收", refusals.Count)
	if refusals.Last != "" {
		detail += "：" + refusals.Last
	}
	return AgentBusSignalView{Kind: "node_rate", Detail: detail}, true
}

// wakeUnreachableSignal turns wakes with nowhere to go into one row. It is deliberately not the
// failure row: those participants are not failing to receive, they are not here at all — a board
// outlives the sessions that wrote to it — and this is the row that would otherwise repeat.
func wakeUnreachableSignal(participants []string) (AgentBusSignalView, bool) {
	if len(participants) == 0 {
		return AgentBusSignalView{}, false
	}
	detail := fmt.Sprintf("板上还有 %d 位不在这台宿主的参与者有待办（已离开或从未公告）", len(participants))
	detail += "：" + strings.Join(participants, "、")
	return AgentBusSignalView{Kind: "wake_unreachable", Detail: detail}, true
}
