package board

import "time"

// applyWaive closes a node as a by-product: nothing verifiable was ever going to come out of it,
// so it carries no evidence at all. It lands in the same terminal state an abandon does — which
// is what keeps it from pinning the container that required it — and it is one phase rather than
// abandon's two, because there is no artifact for a second phase to weigh (F86/F22, 2026-10-05).
func applyWaive(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateDone, StateAbandoned, StateStale:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	if evidencedAsserts(n) > 0 {
		return reject(op.Verb, op.Node, ReasonArtifactPresent)
	}
	n.State = StateAbandoned
	n.Outcome = OutcomeByproduct
	n.OutcomeActor = op.Actor
	n.Owner = ""
	n.Deadline = time.Time{}
	n.LastSeq = op.Seq
	return nil
}

// evidencedAsserts counts the assertions on a node that carry a re-checkable ref: the same test
// done and waive both apply before they let a node close (F45, 2026-10-05).
func evidencedAsserts(n *Node) int {
	count := 0
	for _, a := range n.Asserts {
		if hasEvidence(a.Evidence) {
			count++
		}
	}
	return count
}
