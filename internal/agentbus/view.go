package agentbus

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus/board"
)

// SchemaVersion identifies the machine view's field order and header shape.
const SchemaVersion = "agentbus-view/2"

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
	// Startable is Node.Ready: can run now, which a live claim makes false.
	Startable  bool
	DepsOpen   int
	Evidence   int
	Refuted    bool
	NoProgress int
	LastSeq    uint64
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
		default:
			continue
		}
		if n.Ready(st) {
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
		line := lineFor(st, n)
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

func renderLine(l ViewLine) string {
	return fmt.Sprintf(
		"node id=%s state=%s outcome=%s owner=%s deadline=%s startable=%t deps_open=%d evidence=%d refuted=%t no_progress=%d last_seq=%d title=%q\n",
		l.ID, l.State, l.Outcome, l.Owner, l.Deadline, l.Startable, l.DepsOpen,
		l.Evidence, l.Refuted, l.NoProgress, l.LastSeq, l.Title,
	)
}

func lineFor(st *board.State, n *board.Node) ViewLine {
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
		Startable:  n.Ready(st),
	}
	if !n.Deadline.IsZero() {
		line.Deadline = n.Deadline.UTC().Format(time.RFC3339)
	}
	for _, dep := range n.Deps {
		d := st.Nodes[dep]
		if d == nil || d.State != board.StateDone {
			line.DepsOpen++
		}
	}
	return line
}

// participantNodes are the nodes this participant owns, has asserted on, or was addressed by name
// (assign). The last one matters because a wake tells the assignee "this is yours" — a view that
// then showed nothing would leave the reader with the wake's word and no row behind it.
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
