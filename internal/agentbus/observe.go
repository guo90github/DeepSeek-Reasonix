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
	// AssignedWait is how long work addressed to one participant may sit untaken
	// before it is worth attention: the assignee has not come, and nobody else may.
	AssignedWait time.Duration
}

// Defaults for the first screen.
const (
	DefaultMaxCards   = 12
	DefaultMaxSignals = 40
	// Ten minutes is several missed ticks: a live assignee takes its work within one
	// tick, and the busy-or-gone one is what this window exists to name.
	DefaultAssignedWait = 10 * time.Minute
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
	if lim.AssignedWait <= 0 {
		lim.AssignedWait = DefaultAssignedWait
	}

	folder := &observeFolder{state: state, cards: map[string]*Card{}, assignedWait: lim.AssignedWait}
	folder.board(now)
	folder.deliberations(hearings)
	folder.queue(queue, now)

	briefing := &Briefing{HealthySubtrees: folder.healthySubtrees()}
	attention := folder.attention()
	if len(attention) > lim.MaxCards {
		briefing.HiddenCards = len(attention) - lim.MaxCards
		attention = attention[:lim.MaxCards]
	}
	briefing.Cards = attention
	briefing.Signals, briefing.Hidden = capSignals(folder.signals, lim.MaxSignals)
	return briefing
}

// observeFolder collects cards and signals while the surfaces are folded, so the
// fold reads as one purpose per method instead of one very long function.
type observeFolder struct {
	state        *board.State
	cards        map[string]*Card
	signals      []Signal
	assignedWait time.Duration
}

func (f *observeFolder) card(subtree string) *Card {
	if c, ok := f.cards[subtree]; ok {
		return c
	}
	c := &Card{Subtree: subtree}
	f.cards[subtree] = c
	return c
}

// subtreeOf resolves where a node belongs. Parked work may name a node the board no
// longer holds: then the subtree it was parked under is all we know about it.
func (f *observeFolder) subtreeOf(node, hint string) string {
	if f.state == nil {
		return hint
	}
	if _, ok := f.state.Nodes[node]; !ok && hint != "" {
		return hint
	}
	return SubtreeRoot(f.state, node)
}

func (f *observeFolder) board(now time.Time) {
	if f.state == nil {
		return
	}
	for _, id := range sortedNodeIDs(f.state) {
		n := f.state.Nodes[id]
		subtree := SubtreeRoot(f.state, id)
		c := f.card(subtree)
		c.Nodes++
		switch n.State {
		case board.StateDone:
			c.Done++
		case board.StateClaimed:
			c.AtWork++
		}
		if n.State != board.StateDone && n.State != board.StateAbandoned {
			if detail := orphanReason(f.state, n); detail != "" {
				c.Orphans++
				f.signals = append(f.signals, Signal{Kind: SignalOrphan, Subtree: subtree, Node: id, Detail: detail})
			}
		}
		if detail := stallReason(n, now); detail != "" {
			c.Stalled++
			f.signals = append(f.signals, Signal{Kind: SignalStalled, Subtree: subtree, Node: id, Detail: detail})
		}
		// `contested` is the state machine's own answer to "challenged, not yet ruled on".
		// Counting refutations against an empty Outcome instead would hide a step refuted
		// *after* it was decided: its Outcome still says done (T8-4).
		if n.State == board.StateContested {
			c.Disputed++
			f.signals = append(f.signals, Signal{
				Kind: SignalDisputed, Subtree: subtree, Node: id,
				Detail: fmt.Sprintf("%d refutations, awaiting a verdict", len(n.Refutes)),
			})
		}
	}
}

func (f *observeFolder) deliberations(hearings *HearingState) {
	if hearings == nil {
		return
	}
	for _, node := range sortedHearingNodes(hearings) {
		h := hearings.Hearings[node]
		subtree := f.subtreeOf(node, "")
		c := f.card(subtree)
		switch {
		case h.Open:
			c.Disputed++
			f.signals = append(f.signals, Signal{Kind: SignalDisputed, Subtree: subtree, Node: node, Detail: hearingDetail("under deliberation", h)})
		case h.Verdict == VerdictEscalate:
			f.signals = append(f.signals, Signal{Kind: SignalEscalated, Subtree: subtree, Node: node, Detail: hearingDetail("escalated to a human", h)})
		case h.Verdict == VerdictUndecided:
			f.signals = append(f.signals, Signal{
				Kind: SignalUndecided, Subtree: subtree, Node: node,
				Detail: hearingDetail("closed by rule: nobody may call it settled", h),
			})
		}
	}
}

func (f *observeFolder) queue(queue *QueueState, now time.Time) {
	if queue == nil {
		return
	}
	for _, entry := range queue.Next(0) {
		// Parked work waits for a slot, which is normal: it is a count, not a signal.
		subtree := f.subtreeOf(entry.Node, entry.Subtree)
		f.card(subtree).Parked++
		if detail := f.assignedWaitReason(entry, now); detail != "" {
			c := f.card(subtree)
			c.Stalled++
			f.signals = append(f.signals, Signal{Kind: SignalStalled, Subtree: subtree, Node: entry.Node, Detail: detail})
		}
	}
}

// assignedWaitReason reports work addressed to one participant that nobody took: the
// assignment is what keeps every other claimant out, so this silence is a stall rather
// than the normal wait for a slot (AGENT_BUS §13.4/§13.5).
func (f *observeFolder) assignedWaitReason(entry QueueEntry, now time.Time) string {
	if f.state == nil || f.assignedWait <= 0 || entry.EnqueuedAt.IsZero() {
		return ""
	}
	n := f.state.Nodes[entry.Node]
	if n == nil || n.Assignee == "" {
		return ""
	}
	switch n.State {
	case board.StateDone, board.StateAbandoned, board.StateClaimed:
		return ""
	}
	waiting := now.Sub(entry.EnqueuedAt)
	if waiting < f.assignedWait {
		return ""
	}
	return fmt.Sprintf("assigned to %s, waiting %s with nobody taking it", n.Assignee, waiting.Round(time.Minute))
}

// attribute resolves each card's worst signal and how many it carries.
func (f *observeFolder) attribute() []Card {
	for _, c := range f.cards {
		c.Signals = 0
		c.Worst = ""
	}
	for _, signal := range f.signals {
		c := f.cards[signal.Subtree]
		if c == nil {
			continue
		}
		c.Signals++
		if c.Worst == "" || severity(signal.Kind) < severity(c.Worst) {
			c.Worst = signal.Kind
		}
	}
	out := make([]Card, 0, len(f.cards))
	for _, c := range f.cards {
		if c.Signals == 0 {
			continue
		}
		out = append(out, *c)
	}
	return out
}

// healthySubtrees counts the subtrees that raised no signal at all.
func (f *observeFolder) healthySubtrees() int {
	healthy := 0
	for _, c := range f.cards {
		if c.Signals == 0 && f.cardHasNoSignal(c.Subtree) {
			healthy++
		}
	}
	return healthy
}

func (f *observeFolder) cardHasNoSignal(subtree string) bool {
	for _, signal := range f.signals {
		if signal.Subtree == subtree {
			return false
		}
	}
	return true
}

// attention lists the subtrees that need a card, worst first and busiest next: a
// screen shows what hurts most, not what happens to sort first.
func (f *observeFolder) attention() []Card {
	out := f.attribute()
	sort.Slice(out, func(i, j int) bool {
		if severity(out[i].Worst) != severity(out[j].Worst) {
			return severity(out[i].Worst) < severity(out[j].Worst)
		}
		if out[i].Signals != out[j].Signals {
			return out[i].Signals > out[j].Signals
		}
		return out[i].Subtree < out[j].Subtree
	})
	return out
}

// capSignals keeps every mandatory signal and fills the rest of the room with the
// worst of what is left, counting whatever did not fit instead of dropping it
// silently (T8-2).
func capSignals(signals []Signal, room int) ([]Signal, int) {
	sort.SliceStable(signals, func(i, j int) bool {
		if severity(signals[i].Kind) != severity(signals[j].Kind) {
			return severity(signals[i].Kind) < severity(signals[j].Kind)
		}
		if signals[i].Subtree != signals[j].Subtree {
			return signals[i].Subtree < signals[j].Subtree
		}
		return signals[i].Node < signals[j].Node
	})
	var kept []Signal
	hidden := 0
	for _, signal := range signals {
		if signal.Kind.Mandatory() || room > 0 {
			kept = append(kept, signal)
			if !signal.Kind.Mandatory() {
				room--
			}
			continue
		}
		hidden++
	}
	return kept, hidden
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
	// A settled node is not a live symptom, whatever its history: the no-progress
	// counter it collected while it was stuck stays in the record, but a finished
	// node must never be shown as stalled (T8-2, found by the takeover e2e).
	if n.State == board.StateDone || n.State == board.StateAbandoned {
		return ""
	}
	if n.State == board.StateClaimed && !n.Deadline.IsZero() && now.After(n.Deadline) {
		return "needs handoff: lease lapsed"
	}
	if n.NoProgress > 0 {
		return fmt.Sprintf("no progress recorded %d times", n.NoProgress)
	}
	return ""
}
