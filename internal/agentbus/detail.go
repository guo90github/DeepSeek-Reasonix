package agentbus

import (
	"reasonix/internal/agentbus/board"
)

// Authorization is one approval as a human reads it: who said a node may do what, and why.
type Authorization struct {
	Actor  string
	Reason string
	Seq    uint64
}

// Refutation is one challenge as recorded, so a reader can see what was disputed.
type Refutation struct {
	Actor  string
	Reason string
	Seq    uint64
}

// NodeDetail is one node as the human side reads it: what it is, what it waits for, what is
// in doubt and who authorized it. The authorizations come from the op log, not the fold
// (AGENT_BUS §13.8) — which assertion was a grant is provenance the state cannot carry.
type NodeDetail struct {
	Node           string
	Title          string
	State          board.NodeState
	Owner          string
	Ready          bool
	Deps           []string
	NoProgress     int
	Refutations    []Refutation
	Authorizations []Authorization
	Deliberating   bool
	Verdict        string
}

// DescribeNode reads one node. The second result is false when the board does not hold it,
// so a caller can say "no such step" instead of rendering an empty one.
func DescribeNode(state *board.State, hearings *HearingState, ops []board.Op, node string) (NodeDetail, bool) {
	if state == nil {
		return NodeDetail{}, false
	}
	n, ok := state.Nodes[node]
	if !ok || n == nil {
		return NodeDetail{}, false
	}
	out := NodeDetail{
		Node: n.ID, Title: n.Title, State: n.State, Owner: n.Owner,
		Ready: n.Ready(state), Deps: append([]string(nil), n.Deps...), NoProgress: n.NoProgress,
	}
	for _, r := range n.Refutes {
		out.Refutations = append(out.Refutations, Refutation{Actor: r.Actor, Reason: r.Reason, Seq: r.Seq})
	}
	for _, g := range AuthorizedGrants(ops, state, node) {
		out.Authorizations = append(out.Authorizations, Authorization{Actor: g.Actor, Reason: g.Reason, Seq: g.Seq})
	}
	if hearings != nil {
		if h := hearings.Hearings[node]; h != nil {
			out.Deliberating, out.Verdict = h.Open, h.Verdict
		}
	}
	return out, true
}
