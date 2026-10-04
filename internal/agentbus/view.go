package agentbus

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus/board"
)

// SchemaVersion identifies the machine view's field order and header shape.
const SchemaVersion = "agentbus-view/4"

const (
	defaultMaxLines = 200
	defaultMaxBytes = 8 * 1024
	maxTitleRunes   = 120
)

// ViewSpec bounds one participant's projection of the board.
type ViewSpec struct {
	Board       string
	Participant string
	Cursor      uint64
	MaxLines    int
	MaxBytes    int
}

// ViewLine is one node as a participant may see it. Field order is part of the
// schema: new fields go last so a reader's own prefix stays stable.
type ViewLine struct {
	ID       string
	Title    string
	State    board.NodeState
	Outcome  board.Outcome
	Owner    string
	Deadline string
	// Startable is Node.Ready narrowed by the assignment gate, which is the same gate
	// claim applies; a live claim makes it false too.
	Startable  bool
	DepsOpen   int
	Evidence   int
	Refuted    bool
	NoProgress int
	LastSeq    uint64
	// Assignee is the participant this step is addressed to; empty means the board pool.
	Assignee string
	// Lease is how long the current holder was granted; empty when nobody holds the node.
	Lease string
	// LastOp is the op that last moved this node; its prefix names who wrote it (F64, 2026-10-05).
	LastOp string
}

// View is one participant's bounded projection of the folded board.
type View struct {
	Schema      string
	Board       string
	Participant string
	Cursor      uint64
	Next        uint64
	Truncated   int
	Owned       int
	Waiting     int
	Needed      int
	Ready       int
	Lines       []ViewLine
}

// BuildView projects st for one participant. It is a pure function of the folded
// state: the same state and spec render byte-identically, which is what keeps
// the injected view cacheable. Rows are delivered in ascending last_seq so a
// cursor never skips one; when a cap bites, the remainder waits for the next
// call and is counted, never dropped.
func BuildView(st *board.State, spec ViewSpec) View {
	maxLines, maxBytes := spec.MaxLines, spec.MaxBytes
	if maxLines <= 0 {
		maxLines = defaultMaxLines
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	v := View{
		Schema:      SchemaVersion,
		Board:       spec.Board,
		Participant: spec.Participant,
		Cursor:      spec.Cursor,
	}
	if st == nil {
		return v
	}
	mine := participantNodes(st, spec.Participant)
	candidates := make([]*board.Node, 0, len(st.Nodes))
	for _, id := range sortedNodeIDs(st) {
		n := st.Nodes[id]
		switch {
		case mine[id]:
			v.Owned++
		case waitsOnMine(st, n, mine):
			v.Waiting++
		case neededBy(st, n, mine):
			v.Needed++
		case !requestsNode(n, spec.Participant):
			continue
		}
		if startableFor(st, n, spec.Participant) {
			v.Ready++
		}
		if n.LastSeq > spec.Cursor {
			candidates = append(candidates, n)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].LastSeq != candidates[j].LastSeq {
			return candidates[i].LastSeq < candidates[j].LastSeq
		}
		return candidates[i].ID < candidates[j].ID
	})
	const countersLineReserve = 200
	used := len(v.header()) + countersLineReserve
	for _, n := range candidates {
		line := lineFor(st, n, spec.Participant)
		rendered := len(renderLine(line))
		if len(v.Lines) >= maxLines || used+rendered > maxBytes {
			v.Truncated++
			continue
		}
		v.Lines = append(v.Lines, line)
		used += rendered
		v.Next = n.LastSeq
	}
	if len(v.Lines) == 0 {
		v.Next = spec.Cursor
	}
	return v
}

// Render emits the stable header, the varying counters, then the rows.
func (v View) Render() string {
	var b strings.Builder
	b.WriteString(v.header())
	fmt.Fprintf(&b, "state cursor=%d next=%d owned=%d waiting=%d needed=%d ready=%d truncated=%d\n",
		v.Cursor, v.Next, v.Owned, v.Waiting, v.Needed, v.Ready, v.Truncated)
	for _, line := range v.Lines {
		b.WriteString(renderLine(line))
	}
	return b.String()
}

// header carries only fields that never change for one participant, so a growing
// view stays a prefix-cache hit.
func (v View) header() string {
	return fmt.Sprintf("agentbus view schema=%s board=%s participant=%s\n",
		v.Schema, v.Board, v.Participant)
}

// opSource names who wrote an op, off the only marker the board keeps in the id itself: op- is
// a session's own tool, agentbus-dispatch: the host handing work over, sweep- the board's own
// reclaim (F64/F46, 2026-10-05). The exact id stays in the board's own op log.
func opSource(id string) string {
	switch {
	case strings.HasPrefix(id, "agentbus-dispatch:"):
		return "dispatch"
	case strings.HasPrefix(id, "sweep-"):
		return "sweep"
	case strings.HasPrefix(id, "op-"):
		return "hand"
	default:
		return "other"
	}
}

func renderLine(l ViewLine) string {
	// Both suffixes are conditional so a row without them stays byte-identical to what it was
	// before they existed: most rows are unheld, and they are what the byte cap and the prompt
	// cache see (2026-10-05).
	suffix := ""
	if l.Assignee != "" {
		suffix += " assignee=" + l.Assignee
	}
	if l.Lease != "" {
		suffix += " lease=" + l.Lease
	}
	if l.LastOp != "" {
		suffix += " op=" + opSource(l.LastOp)
	}
	// asserts=, not evidence=: the number counts assertions on the node, and calling a count
	// "evidence" made a probe with twenty empty assertions read as well-evidenced (F45, 2026-10-05).
	return fmt.Sprintf(
		"node id=%s state=%s outcome=%s owner=%s deadline=%s startable=%t deps_open=%d asserts=%d refuted=%t no_progress=%d last_seq=%d title=%q%s\n",
		l.ID, l.State, l.Outcome, l.Owner, l.Deadline, l.Startable, l.DepsOpen,
		l.Evidence, l.Refuted, l.NoProgress, l.LastSeq, l.Title, suffix,
	)
}

func lineFor(st *board.State, n *board.Node, participant string) ViewLine {
	line := ViewLine{
		ID:         n.ID,
		Title:      truncateRunes(n.Title, maxTitleRunes),
		State:      n.State,
		Outcome:    n.Outcome,
		Owner:      n.Owner,
		Evidence:   len(n.Asserts),
		Refuted:    len(n.Refutes) > 0,
		NoProgress: n.NoProgress,
		LastSeq:    n.LastSeq,
		Startable:  startableFor(st, n, participant),
		Assignee:   n.Assignee,
		LastOp:     n.LastOpID,
	}
	if !n.Deadline.IsZero() {
		line.Deadline = n.Deadline.UTC().Format(time.RFC3339)
	}
	// The lease length is what tells one way of claiming from another — a hand-written claim
	// defaults to 900s and the host's dispatch to 1800s — and the row used to carry only the
	// deadline, so a reader could not tell which it was (F52, 2026-10-05).
	if n.State == board.StateClaimed && !n.ClaimedAt.IsZero() {
		line.Lease = n.Deadline.Sub(n.ClaimedAt).Round(time.Second).String()
	}
	for _, dep := range n.Deps {
		if !board.DepSettled(st.Nodes[dep]) {
			line.DepsOpen++
		}
	}
	return line
}

// startableFor reports whether this step can run for this participant: the graph's own
// readiness, narrowed by the assignment gate claim applies. Reporting Node.Ready alone
// told a reader a step addressed to somebody else was startable, while claim refused it
// (2026-10-05).
func startableFor(st *board.State, n *board.Node, participant string) bool {
	if n.Assignee != "" && n.Assignee != participant {
		return false
	}
	return n.Ready(st)
}

// participantNodes are the nodes this participant holds or has asserted on. Being addressed a
// node (assign) counts as holding it: a wake tells the assignee "this is yours" — a view that
// then showed nothing would leave the reader with the wake's word and no row behind it.
// Requesters are not in here: asking for a node is not holding it, so they are visible through
// requestsNode without inflating the owned count.
func participantNodes(st *board.State, participant string) map[string]bool {
	out := map[string]bool{}
	if participant == "" {
		return out
	}
	for id, n := range st.Nodes {
		if n.Owner == participant || n.Assignee == participant {
			out[id] = true
			continue
		}
		for _, a := range n.Asserts {
			if a.Actor == participant {
				out[id] = true
				break
			}
		}
	}
	return out
}

// requestsNode reports whether this participant asked for n: whichever assert, split or
// require brought it into being, plus every later require that named it. The wake names those
// same participants ("startable now"), and dropping their rows left a session unable to read
// anything its own structure had created (measured 2026-10-05: a splitter's view held none of
// its own children, so it could not read the deps it had just written).
func requestsNode(n *board.Node, participant string) bool {
	if participant == "" {
		return false
	}
	for _, requester := range n.Requesters {
		if requester == participant {
			return true
		}
	}
	return false
}

// waitsOnMine reports whether n sits in the reverse closure of my nodes and is
// still waiting — the "who is blocked on me" half of the view.
func waitsOnMine(st *board.State, n *board.Node, mine map[string]bool) bool {
	if n.State == board.StateDone || n.State == board.StateAbandoned {
		return false
	}
	for target := range mine {
		if target == n.ID {
			continue
		}
		if dependsOn(st, n.ID, target) {
			return true
		}
	}
	return false
}

// neededBy reports whether n is a live dependency of one of my nodes — the
// "what am I waiting for" half of the view. A stale dependency stays visible:
// it is exactly the basis that changed under me.
func neededBy(st *board.State, n *board.Node, mine map[string]bool) bool {
	if n.State == board.StateDone || n.State == board.StateAbandoned {
		return false
	}
	for id := range mine {
		if id == n.ID {
			continue
		}
		if dependsOn(st, id, n.ID) {
			return true
		}
	}
	return false
}

// dependsOn walks n's dependencies looking for target.
func dependsOn(st *board.State, id, target string) bool {
	seen := map[string]bool{}
	stack := []string{id}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == target {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if n := st.Nodes[cur]; n != nil {
			stack = append(stack, n.Deps...)
		}
	}
	return false
}

func sortedNodeIDs(st *board.State) []string {
	out := make([]string, 0, len(st.Nodes))
	for id := range st.Nodes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
