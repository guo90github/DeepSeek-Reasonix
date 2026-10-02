package board

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// NodeState is the closed set of node states.
type NodeState string

const (
	StateOpen          NodeState = "open"
	StateClaimed       NodeState = "claimed"
	StateContested     NodeState = "contested"
	StateDone          NodeState = "done"
	StateBlocked       NodeState = "blocked"
	StateAbandoned     NodeState = "abandoned"
	StateStale         NodeState = "stale"
	StateCapabilityGap NodeState = "capability_gap"
)

// Assertion is one recorded assert on a node.
type Assertion struct {
	Actor    string     `json:"actor"`
	At       time.Time  `json:"at"`
	Seq      uint64     `json:"seq"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Summary  string     `json:"summary,omitempty"`
}

// Refutation is one recorded refute on a node.
type Refutation struct {
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
	Seq    uint64    `json:"seq"`
	Reason string    `json:"reason"`
}

// AbandonRequest is an evidenced request that survives until decide rules on it.
type AbandonRequest struct {
	Actor    string     `json:"actor"`
	At       time.Time  `json:"at"`
	Seq      uint64     `json:"seq"`
	Reason   string     `json:"reason"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

// Node is one unit of work as the folded log sees it.
type Node struct {
	ID           string          `json:"id"`
	Title        string          `json:"title,omitempty"`
	State        NodeState       `json:"state"`
	Deps         []string        `json:"deps,omitempty"`
	Owner        string          `json:"owner,omitempty"`
	Deadline     time.Time       `json:"deadline,omitzero"`
	Bounds       *Bounds         `json:"bounds,omitempty"`
	Asserts      []Assertion     `json:"asserts,omitempty"`
	Refutes      []Refutation    `json:"refutes,omitempty"`
	AbandonReq   *AbandonRequest `json:"abandonReq,omitempty"`
	Outcome      Outcome         `json:"outcome,omitempty"`
	OutcomeActor string          `json:"outcomeActor,omitempty"`
	NoProgress   int             `json:"noProgress,omitempty"`
	LastSeq      uint64          `json:"lastSeq"`
}

// Ready reports the derived readiness predicate: the node may start once every
// dependency is done and no verdict blocks it. A split container becomes ready
// again when its children finish, because assembling them is its own work.
func (n *Node) Ready(st *State) bool {
	if n.Outcome == OutcomeBlocked {
		return false
	}
	switch n.State {
	case StateOpen, StateBlocked:
	default:
		return false
	}
	for _, dep := range n.Deps {
		d := st.Nodes[dep]
		if d == nil || d.State != StateDone {
			return false
		}
	}
	return true
}

// claimable reports whether a node may be taken: a fresh node, a waiting node
// whose dependencies are all done, or one whose capability gap was closed.
// Decided-blocked and stale nodes stay out: they need a verdict or a revert.
func claimable(st *State, n *Node) bool {
	switch n.State {
	case StateOpen, StateCapabilityGap:
		return true
	case StateBlocked:
		return n.Ready(st)
	default:
		return false
	}
}

// ClaimExpired reports the derived expiry predicate for a live claim.
func (n *Node) ClaimExpired(now time.Time) bool {
	return n.State == StateClaimed && !n.Deadline.IsZero() && now.After(n.Deadline)
}

func claimExpired(n *Node, now time.Time) bool { return n.ClaimExpired(now) }

func (st *State) nodeOrCreate(id string) *Node {
	n := st.Nodes[id]
	if n == nil {
		n = &Node{ID: id, State: StateOpen}
		st.Nodes[id] = n
	}
	return n
}

// dependsTransitively reports whether from reaches target along dependsOn edges.
func (st *State) dependsTransitively(from, target string) bool {
	seen := map[string]bool{}
	stack := []string{from}
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

func (st *State) sortedNodeIDs() []string {
	out := make([]string, 0, len(st.Nodes))
	for id := range st.Nodes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (st *State) containsDep(n *Node, dep string) bool {
	return slices.Contains(n.Deps, dep)
}

// validateOpShape refuses an op whose own fields are malformed. That is a
// property of the op rather than of the node it names, so it runs before any
// state lookup and reports the reason the caller can actually act on.
func validateOpShape(op Op) error {
	if err := requireNode(op); err != nil {
		return err
	}
	switch op.Verb {
	case VerbAssert:
		if !hasEvidence(op.Evidence) {
			return reject(op.Verb, op.Node, ReasonMissingEvidence)
		}
	case VerbRefute:
		if strings.TrimSpace(op.Reason) == "" {
			return reject(op.Verb, op.Node, ReasonMissingReason)
		}
	case VerbClaim:
		if strings.TrimSpace(op.Actor) == "" {
			return reject(op.Verb, op.Node, ReasonMissingActor)
		}
		if op.Deadline.IsZero() {
			return reject(op.Verb, op.Node, ReasonMissingDeadline)
		}
		if op.Bounds == nil {
			return reject(op.Verb, op.Node, ReasonMissingBounds)
		}
	case VerbHeartbeat:
		if strings.TrimSpace(op.Actor) == "" {
			return reject(op.Verb, op.Node, ReasonMissingActor)
		}
		if op.Deadline.IsZero() {
			return reject(op.Verb, op.Node, ReasonMissingDeadline)
		}
	case VerbCapabilityGap:
		if strings.TrimSpace(op.Reason) == "" {
			return reject(op.Verb, op.Node, ReasonMissingReason)
		}
	case VerbAbandon:
		if strings.TrimSpace(op.Reason) == "" {
			return reject(op.Verb, op.Node, ReasonMissingReason)
		}
		if !hasEvidence(op.Evidence) {
			return reject(op.Verb, op.Node, ReasonMissingEvidence)
		}
	case VerbSplit:
		if len(op.Children) == 0 {
			return reject(op.Verb, op.Node, ReasonMissingDependency)
		}
		seen := make(map[string]bool, len(op.Children))
		for _, child := range op.Children {
			id := strings.TrimSpace(child.ID)
			if id == "" {
				return reject(op.Verb, op.Node, ReasonMissingDependency)
			}
			if seen[id] {
				return reject(op.Verb, id, ReasonDuplicateDependency)
			}
			seen[id] = true
		}
	case VerbRequire:
		if op.Dep == nil || strings.TrimSpace(op.Dep.ID) == "" {
			return reject(op.Verb, op.Node, ReasonMissingDependency)
		}
	case VerbDecide:
		switch op.Outcome {
		case OutcomeDone, OutcomeBlocked, OutcomeAbandoned:
		default:
			return reject(op.Verb, op.Node, ReasonUnknownOutcome)
		}
	case VerbRelease, VerbYield, VerbRevert, VerbNoProgress:
	default:
		return reject(op.Verb, op.Node, ReasonUnknownVerb)
	}
	return nil
}

func requireNode(op Op) error {
	if strings.TrimSpace(op.Node) == "" {
		return reject(op.Verb, op.Node, ReasonMissingNode)
	}
	return nil
}

// applyOp mutates st with op, or refuses with a typed reason. Fold and the write
// path share this one implementation, and it reads no clock: every transition it
// stores is a function of the log alone, so replay cannot drift with read time.
func applyOp(st *State, op Op) error {
	if err := validateOpShape(op); err != nil {
		return err
	}
	switch op.Verb {
	case VerbAssert:
		return applyAssert(st, op)
	case VerbRefute:
		return applyRefute(st, op)
	case VerbClaim:
		return applyClaim(st, op)
	case VerbHeartbeat:
		return applyHeartbeat(st, op)
	case VerbRelease, VerbYield:
		return applyRelease(st, op)
	case VerbSplit:
		return applySplit(st, op)
	case VerbRequire:
		return applyRequire(st, op)
	case VerbCapabilityGap:
		return applyCapabilityGap(st, op)
	case VerbAbandon:
		return applyAbandon(st, op)
	case VerbDecide:
		return applyDecide(st, op)
	case VerbRevert:
		return applyRevert(st, op)
	case VerbNoProgress:
		return applyNoProgress(st, op)
	default:
		return reject(op.Verb, op.Node, ReasonUnknownVerb)
	}
}

func applyAssert(st *State, op Op) error {
	n := st.nodeOrCreate(op.Node)
	switch n.State {
	case StateDone, StateAbandoned, StateStale:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	n.Asserts = append(n.Asserts, Assertion{
		Actor: op.Actor, At: op.At, Seq: op.Seq, Evidence: op.Evidence, Summary: op.Reason,
	})
	if len(n.Refutes) > 0 {
		n.State = StateContested
	}
	n.LastSeq = op.Seq
	return nil
}

func applyRefute(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateAbandoned, StateStale:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	n.Refutes = append(n.Refutes, Refutation{Actor: op.Actor, At: op.At, Seq: op.Seq, Reason: op.Reason})
	n.State = StateContested
	n.LastSeq = op.Seq
	return nil
}

func applyClaim(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	// Taking over is legal only when the previous lease had already lapsed at
	// the moment this op was written: the comparison uses the log's own
	// timestamps, so it replays identically anywhere.
	lapsed := n.State == StateClaimed && op.At.After(n.Deadline)
	if !claimable(st, n) && !lapsed {
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	n.State = StateClaimed
	n.Owner = op.Actor
	n.Deadline = op.Deadline.UTC()
	n.Bounds = op.Bounds
	n.LastSeq = op.Seq
	return nil
}

func applyHeartbeat(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	if n.State != StateClaimed {
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	if op.Actor != n.Owner {
		return reject(op.Verb, op.Node, ReasonNotOwner)
	}
	n.Deadline = op.Deadline.UTC()
	n.LastSeq = op.Seq
	return nil
}

func applyRelease(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	if n.State != StateClaimed {
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	if op.Actor != n.Owner {
		return reject(op.Verb, op.Node, ReasonNotOwner)
	}
	n.State = StateOpen
	n.Owner = ""
	n.Deadline = time.Time{}
	n.LastSeq = op.Seq
	return nil
}

func applySplit(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateDone, StateAbandoned:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	for _, child := range op.Children {
		if _, exists := st.Nodes[child.ID]; exists {
			return reject(op.Verb, child.ID, ReasonDuplicateNode)
		}
		if st.containsDep(n, child.ID) {
			return reject(op.Verb, child.ID, ReasonDuplicateDependency)
		}
		if st.dependsTransitively(n.ID, child.ID) {
			return reject(op.Verb, child.ID, ReasonCycle)
		}
	}
	for _, child := range op.Children {
		created := st.nodeOrCreate(child.ID)
		created.Title = child.Title
		created.LastSeq = op.Seq
		n.Deps = append(n.Deps, child.ID)
	}
	n.State = StateBlocked
	n.LastSeq = op.Seq
	return nil
}

func applyRequire(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateDone, StateAbandoned:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	depID := op.Dep.ID
	if st.containsDep(n, depID) {
		return reject(op.Verb, depID, ReasonDuplicateDependency)
	}
	if depID == n.ID || st.dependsTransitively(depID, n.ID) {
		return reject(op.Verb, depID, ReasonCycle)
	}
	if dep := st.Nodes[depID]; dep != nil {
		if dep.State == StateAbandoned {
			return reject(op.Verb, depID, ReasonDependencyClosed)
		}
		if op.Dep.Title != "" && dep.Title == "" {
			dep.Title = op.Dep.Title
		}
	} else {
		created := st.nodeOrCreate(depID)
		created.Title = op.Dep.Title
		created.LastSeq = op.Seq
	}
	n.Deps = append(n.Deps, depID)
	n.State = StateBlocked
	n.LastSeq = op.Seq
	return nil
}

func applyCapabilityGap(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateOpen, StateClaimed:
	default:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	n.State = StateCapabilityGap
	n.Owner = ""
	n.Deadline = time.Time{}
	n.LastSeq = op.Seq
	return nil
}

func applyAbandon(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateDone, StateAbandoned:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	n.AbandonReq = &AbandonRequest{
		Actor: op.Actor, At: op.At, Seq: op.Seq, Reason: op.Reason, Evidence: op.Evidence,
	}
	n.LastSeq = op.Seq
	return nil
}

func applyDecide(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch op.Outcome {
	case OutcomeDone:
		switch n.State {
		case StateAbandoned, StateStale:
			return reject(op.Verb, op.Node, ReasonIllegalTransition)
		}
		producers := map[string]bool{}
		evidenced := 0
		for _, a := range n.Asserts {
			if hasEvidence(a.Evidence) {
				evidenced++
			}
			producers[a.Actor] = true
		}
		if evidenced == 0 {
			return reject(op.Verb, op.Node, ReasonMissingEvidence)
		}
		if strings.TrimSpace(op.ReproducedBy) == "" {
			return reject(op.Verb, op.Node, ReasonMissingReproducer)
		}
		if producers[op.ReproducedBy] {
			return reject(op.Verb, op.Node, ReasonSelfReproduced)
		}
		n.State = StateDone
		n.Outcome = OutcomeDone
		n.OutcomeActor = op.Actor
		n.Owner = ""
		n.Deadline = time.Time{}
	case OutcomeBlocked:
		switch n.State {
		case StateOpen, StateClaimed, StateContested:
		default:
			return reject(op.Verb, op.Node, ReasonInvalidOutcomeForState)
		}
		n.State = StateBlocked
		n.Outcome = OutcomeBlocked
		n.OutcomeActor = op.Actor
		n.Owner = ""
		n.Deadline = time.Time{}
	case OutcomeAbandoned:
		if n.AbandonReq == nil || !hasEvidence(n.AbandonReq.Evidence) {
			return reject(op.Verb, op.Node, ReasonMissingAbandonRequest)
		}
		n.State = StateAbandoned
		n.Outcome = OutcomeAbandoned
		n.OutcomeActor = op.Actor
		n.Owner = ""
		n.Deadline = time.Time{}
	default:
		return reject(op.Verb, op.Node, ReasonUnknownOutcome)
	}
	n.LastSeq = op.Seq
	return nil
}

func applyRevert(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	switch n.State {
	case StateDone, StateStale:
	default:
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	downstream := make([]string, 0, len(st.Nodes))
	for _, id := range st.sortedNodeIDs() {
		if id == n.ID {
			continue
		}
		if st.dependsTransitively(id, n.ID) {
			downstream = append(downstream, id)
		}
	}
	n.State = StateOpen
	n.Outcome = ""
	n.OutcomeActor = ""
	n.Owner = ""
	n.Deadline = time.Time{}
	n.LastSeq = op.Seq
	for _, id := range downstream {
		d := st.Nodes[id]
		if d != nil && d.State == StateDone {
			d.State = StateStale
			d.Outcome = ""
			d.OutcomeActor = ""
			d.LastSeq = op.Seq
		}
	}
	return nil
}

func applyNoProgress(st *State, op Op) error {
	n := st.Nodes[op.Node]
	if n == nil {
		return reject(op.Verb, op.Node, ReasonUnknownNode)
	}
	if op.Actor != ActorSystem {
		return reject(op.Verb, op.Node, ReasonNotOwner)
	}
	if n.State != StateClaimed || !op.At.After(n.Deadline) {
		return reject(op.Verb, op.Node, ReasonIllegalTransition)
	}
	n.State = StateOpen
	n.Owner = ""
	n.Deadline = time.Time{}
	n.NoProgress++
	n.LastSeq = op.Seq
	return nil
}

// ReclaimOp builds the system record that reclaims one expired claim.
func ReclaimOp(n *Node, now time.Time) Op {
	return Op{
		ID:     SweepID(n.ID, n.Deadline),
		Verb:   VerbNoProgress,
		Node:   n.ID,
		Actor:  ActorSystem,
		At:     now,
		Reason: fmt.Sprintf("claim by %q expired at %s", n.Owner, n.Deadline.UTC().Format(time.RFC3339)),
	}
}
