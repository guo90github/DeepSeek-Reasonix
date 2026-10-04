package agentbus

import (
	"fmt"
	"sort"
	"strings"

	"reasonix/internal/agentbus/board"
)

// BlockerKind is the closed set of reasons a task has not landed.
type BlockerKind string

const (
	// BlockerNotDone names a deliverable, or something it rests on, that is not done.
	BlockerNotDone BlockerKind = "not_done"
	// BlockerMissing names a dependency the board does not hold: it can never be done.
	BlockerMissing BlockerKind = "missing"
	// BlockerAbandoned names work nobody will finish: it needs a revert or a replan.
	BlockerAbandoned BlockerKind = "abandoned"
	// BlockerContested names a conclusion that was challenged and never ruled on.
	BlockerContested BlockerKind = "contested"
	// BlockerDeliberating names a question a hearing still has open.
	BlockerDeliberating BlockerKind = "deliberating"
	// BlockerEscalated names a verdict sent to a human that has not come back.
	BlockerEscalated BlockerKind = "escalated"
	// BlockerUndecided names a question closed by rule: nobody may call it settled.
	BlockerUndecided BlockerKind = "undecided"
)

// Blocker is one reason the task has not landed, with the address to drill into.
type Blocker struct {
	Kind   BlockerKind
	Node   string
	Detail string
}

// Landing is the task-level verdict. Acceptance is structural (AGENT_BUS §13.10): the
// nodes nothing else depends on are the deliverables, so those are the ones that have to
// be done. A board whose deliverables are all done has still not landed while any
// conclusion is in doubt — the two conditions are checked, not traded.
type Landing struct {
	Landed   bool
	Reason   string
	Blockers []Blocker
}

// AssessLanding folds the board and the deliberations into that verdict. It collects
// every reason it finds rather than the first: "why is this not done" is the question a
// human actually asks, and answering it in one entry hides the rest.
func AssessLanding(state *board.State, hearings *HearingState) *Landing {
	out := &Landing{}
	if state == nil || len(state.Nodes) == 0 {
		out.Reason = "the board is empty: nothing has been claimed as the task"
		return out
	}

	found := map[string]Blocker{}
	remember := func(b Blocker) {
		if prev, ok := found[b.Node]; ok && blockerPriority(prev.Kind) <= blockerPriority(b.Kind) {
			return
		}
		found[b.Node] = b
	}

	for _, root := range rootsOf(state) {
		for _, id := range dependencyClosure(state, root) {
			n := state.Nodes[id]
			switch {
			case n == nil:
				remember(Blocker{Kind: BlockerMissing, Node: id, Detail: "a dependency the board does not hold"})
			case n.State == board.StateDone:
			case n.State == board.StateAbandoned:
				remember(Blocker{Kind: BlockerAbandoned, Node: id, Detail: "abandoned: this needs a revert or a replan"})
			default:
				remember(Blocker{Kind: BlockerNotDone, Node: id, Detail: fmt.Sprintf("state %s", n.State)})
			}
		}
	}

	// `contested` is the state machine's own answer to "challenged and not ruled on":
	// observe's "refutations, no verdict" rule reads the fold instead and misses a node
	// refuted *after* it was decided, whose Outcome still says done.
	for _, id := range sortedNodeIDs(state) {
		n := state.Nodes[id]
		if n.State != board.StateContested {
			continue
		}
		remember(Blocker{
			Kind: BlockerContested, Node: id,
			Detail: fmt.Sprintf("%d refutations, no verdict", len(n.Refutes)),
		})
	}

	if hearings != nil {
		for _, node := range sortedHearingNodes(hearings) {
			h := hearings.Hearings[node]
			switch {
			case h.Open:
				remember(Blocker{Kind: BlockerDeliberating, Node: node, Detail: hearingDetail("under deliberation", h)})
			case h.Verdict == VerdictEscalate:
				remember(Blocker{Kind: BlockerEscalated, Node: node, Detail: hearingDetail("escalated to a human", h)})
			case h.Verdict == VerdictUndecided:
				remember(Blocker{Kind: BlockerUndecided, Node: node, Detail: hearingDetail("closed by rule: nobody may call it settled", h)})
			}
		}
	}

	for _, id := range sortedKeys(found) {
		out.Blockers = append(out.Blockers, found[id])
	}
	out.Landed = len(out.Blockers) == 0
	out.Reason = landingReason(state, out)
	return out
}

// blockerPriority orders the reasons a single node may carry, so the most actionable one
// is what that node reports: a live dispute is worth saying before "not done".
func blockerPriority(kind BlockerKind) int {
	switch kind {
	case BlockerContested, BlockerDeliberating:
		return 0
	case BlockerEscalated, BlockerUndecided:
		return 1
	case BlockerMissing:
		return 2
	case BlockerAbandoned:
		return 3
	default:
		return 4
	}
}

// rootsOf lists the nodes nothing depends on: with the tree grown by require and split,
// those are the deliverables themselves.
func rootsOf(state *board.State) []string {
	depended := map[string]bool{}
	ids := sortedNodeIDs(state)
	for _, id := range ids {
		for _, dep := range state.Nodes[id].Deps {
			depended[dep] = true
		}
	}
	out := []string{}
	for _, id := range ids {
		if !depended[id] {
			out = append(out, id)
		}
	}
	return out
}

// dependencyClosure collects a node and everything it rests on. Missing nodes stay in the
// result: a dependency the board does not hold is exactly what must be reported.
func dependencyClosure(state *board.State, root string) []string {
	seen := map[string]bool{}
	stack := []string{root}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		if n := state.Nodes[id]; n != nil {
			stack = append(stack, n.Deps...)
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(found map[string]Blocker) []string {
	out := make([]string, 0, len(found))
	for id := range found {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// landingReason says in one line why the task has not landed, counted by reason.
func landingReason(state *board.State, out *Landing) string {
	if out.Landed {
		return fmt.Sprintf("landed: all %d nodes are done and nothing is in doubt", len(state.Nodes))
	}
	counts := map[BlockerKind]int{}
	for _, b := range out.Blockers {
		counts[b.Kind]++
	}
	parts := []string{}
	for _, kind := range []BlockerKind{
		BlockerContested, BlockerDeliberating, BlockerEscalated, BlockerUndecided,
		BlockerMissing, BlockerAbandoned, BlockerNotDone,
	} {
		if counts[kind] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[kind], kind))
		}
	}
	return "not landed: " + strings.Join(parts, ", ")
}
