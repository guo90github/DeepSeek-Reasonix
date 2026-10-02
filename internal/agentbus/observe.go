package agentbus

import (
	"fmt"
	"sort"
	"time"

	"reasonix/internal/agentbus/board"
)

// SignalKind is the closed set of things worth a human's attention: anomalies,
// disputes, stalls and orphans. Healthy progress is a number on a card, never a
// signal — a screen that shows everything shows nothing (AGENT_BUS §7).
type SignalKind string

const (
	SignalOrphan    SignalKind = "orphan"
	SignalStalled   SignalKind = "stalled"
	SignalEscalated SignalKind = "escalated"
	SignalDisputed  SignalKind = "disputed"
	SignalUndecided SignalKind = "undecided"
)

// severity orders signals for the first screen. The first two are mandatory: an
// orphan cannot ever start and a stall means nobody will notice on their own, so
// neither may be crowded out by the cap (T8-2).
func severity(kind SignalKind) int {
	switch kind {
	case SignalOrphan:
		return 0
	case SignalStalled:
		return 1
	case SignalEscalated:
		return 2
	case SignalDisputed:
		return 3
	default:
		return 4
	}
}

// Mandatory reports whether a signal must be shown whatever the cap says.
func (kind SignalKind) Mandatory() bool {
	return kind == SignalOrphan || kind == SignalStalled
}

// ObserveLimits tunes the first screen. Zero keeps the defaults.
type ObserveLimits struct {
	// MaxCards is how many subtree cards the first screen draws.
	MaxCards int
	// MaxSignals is how many signals it carries. Mandatory signals are never
	// dropped to satisfy it.
	MaxSignals int
}

// Defaults for the first screen.
const (
	DefaultMaxCards   = 12
	DefaultMaxSignals = 40
)

// Signal is one thing to look at, carrying the address needed to drill in.
type Signal struct {
	Kind    SignalKind
	Subtree string
	Node    string
	Detail  string
}

// Card is one subtree that has something wrong with it, folded to counts.
type Card struct {
	Subtree  string
	Nodes    int
	AtWork   int
	Parked   int
	Done     int
	Worst    SignalKind
	Signals  int
	Disputed int
	Stalled  int
	Orphans  int
}

// Briefing is the folded picture the human side reads: cards for the subtrees that
// need attention, the signals to drill into, and honest counts of what did not fit.
type Briefing struct {
	Cards           []Card
	Signals         []Signal
	Hidden          int
	HiddenCards     int
	HealthySubtrees int
}

// Observe folds the board, the queue and the deliberations into what deserves
// attention. Only subtrees with a signal get a card; a healthy subtree is a number.
func Observe(state *board.State, queue *QueueState, hearings *HearingState, now time.Time, lim ObserveLimits) *Briefing {
	if lim.MaxCards <= 0 {
		lim.MaxCards = DefaultMaxCards
	}
	if lim.MaxSignals <= 0 {
		lim.MaxSignals = DefaultMaxSignals
	}

	cards := map[string]*Card{}
	card := func(subtree string) *Card {
		if c, ok := cards[subtree]; ok {
			return c
		}
		c := &Card{Subtree: subtree, Worst: SignalUndecided}
		cards[subtree] = c
		return c
	}
	signals := []Signal{}
	add := func(kind SignalKind, subtree, node, detail string) {
		signals = append(signals, Signal{Kind: kind, Subtree: subtree, Node: node, Detail: detail})
	}
	subtreeOf := func(node, hint string) string {
		if state == nil {
			return hint
		}
		// Parked work may name a node the board no longer holds: then the subtrees it
		// was parked under is the only thing we know about it.
		if _, ok := state.Nodes[node]; !ok && hint != "" {
			return hint
		}
		return SubtreeRoot(state, node)
	}

	if state != nil {
		for _, id := range sortedNodeIDs(state) {
			n := state.Nodes[id]
			subtree := SubtreeRoot(state, id)
			c := card(subtree)
			c.Nodes++
			switch n.State {
			case board.StateDone:
				c.Done++
			case board.StateClaimed:
				c.AtWork++
			}
			if n.State != board.StateDone && n.State != board.StateAbandoned {
				if detail := orphanReason(state, n); detail != "" {
					c.Orphans++
					add(SignalOrphan, subtree, id, detail)
				}
			}
			if detail := stallReason(n, now); detail != "" {
				c.Stalled++
				add(SignalStalled, subtree, id, detail)
			}
			if len(n.Refutes) > 0 && n.Outcome == "" {
				c.Disputed++
				add(SignalDisputed, subtree, id, fmt.Sprintf("%d refutations, no verdict", len(n.Refutes)))
			}
		}
	}

	if hearings != nil {
		for _, node := range sortedHearingNodes(hearings) {
			h := hearings.Hearings[node]
			subtree := subtreeOf(node, "")
			c := card(subtree)
			switch {
			case h.Open:
				c.Disputed++
				add(SignalDisputed, subtree, node, "under deliberation")
			case h.Verdict == VerdictEscalate:
				add(SignalEscalated, subtree, node, "escalated to a human")
			case h.Verdict == VerdictUndecided:
				add(SignalUndecided, subtree, node, "closed by rule: nobody may call it settled")
			default:
				continue
			}
		}
	}

	if queue != nil {
		for _, entry := range queue.Next(0) {
			// Parked work is waiting for a slot, which is normal: it is a count, not
			// a signal. Only work that cannot ever run earns a card.
			card(subtreeOf(entry.Node, entry.Subtree)).Parked++
		}
	}

	digest := &Briefing{}
	for _, c := range cards {
		c.Signals = 0
		c.Worst = ""
	}
	for _, signal := range signals {
		c := cards[signal.Subtree]
		if c == nil {
			continue
		}
		c.Signals++
		if c.Worst == "" || severity(signal.Kind) < severity(c.Worst) {
			c.Worst = signal.Kind
		}
	}

	attention := make([]Card, 0, len(cards))
	for _, c := range cards {
		if c.Signals == 0 {
			digest.HealthySubtrees++
			continue
		}
		attention = append(attention, *c)
	}
	sort.Slice(attention, func(i, j int) bool {
		if severity(attention[i].Worst) != severity(attention[j].Worst) {
			return severity(attention[i].Worst) < severity(attention[j].Worst)
		}
		if attention[i].Signals != attention[j].Signals {
			return attention[i].Signals > attention[j].Signals
		}
		return attention[i].Subtree < attention[j].Subtree
	})
	if len(attention) > lim.MaxCards {
		digest.HiddenCards = len(attention) - lim.MaxCards
		attention = attention[:lim.MaxCards]
	}
	digest.Cards = attention

	sort.SliceStable(signals, func(i, j int) bool {
		if severity(signals[i].Kind) != severity(signals[j].Kind) {
			return severity(signals[i].Kind) < severity(signals[j].Kind)
		}
		if signals[i].Subtree != signals[j].Subtree {
			return signals[i].Subtree < signals[j].Subtree
		}
		return signals[i].Node < signals[j].Node
	})
	room := lim.MaxSignals
	for _, signal := range signals {
		if signal.Kind.Mandatory() || room > 0 {
			digest.Signals = append(digest.Signals, signal)
			if !signal.Kind.Mandatory() {
				room--
			}
			continue
		}
		digest.Hidden++
	}
	return digest
}

// orphanReason reports work that can never start: a node whose dependency is gone,
// either deleted or abandoned. Such a node waits forever unless someone is told.
func orphanReason(state *board.State, n *board.Node) string {
	for _, dep := range n.Deps {
		d, ok := state.Nodes[dep]
		if !ok {
			return fmt.Sprintf("depends on %s, which does not exist", dep)
		}
		if d.State == board.StateAbandoned {
			return fmt.Sprintf("depends on %s, which was abandoned", dep)
		}
	}
	return ""
}

// stallReason reports work that is stopped and will not restart on its own: a lapsed
// lease, or a node the sweeper has already recorded no progress for.
func stallReason(n *board.Node, now time.Time) string {
	if n.State == board.StateClaimed && !n.Deadline.IsZero() && now.After(n.Deadline) {
		return "needs handoff: lease lapsed"
	}
	if n.NoProgress > 0 {
		return fmt.Sprintf("no progress recorded %d times", n.NoProgress)
	}
	return ""
}
