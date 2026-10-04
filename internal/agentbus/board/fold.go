package board

import "time"

// State is the folded blackboard: nodes plus the counters that keep a lossy
// read honest.
type State struct {
	Nodes     map[string]*Node  `json:"nodes"`
	Seq       uint64            `json:"seq"`
	OpIDs     map[string]uint64 `json:"-"`
	Truncated int               `json:"truncated,omitempty"`
	Skipped   int               `json:"skipped,omitempty"`
	Rejected  int               `json:"rejected,omitempty"`
	Applied   int               `json:"applied,omitempty"`
}

// NewState returns an empty folded state.
func NewState() *State {
	return &State{Nodes: map[string]*Node{}, OpIDs: map[string]uint64{}}
}

// Fold replays ops in log order into a state. It reads no clock: every stored
// transition is a function of the log alone, so the same ops fold identically at
// any moment and in any process. Time-derived predicates (expiry, readiness) are
// computed by the caller from the folded nodes.
func Fold(ops []Op) *State {
	return foldFrom(NewState(), ops)
}

// foldFrom replays ops into an existing folded state, so a checkpoint can be extended instead of
// rebuilt. It is Fold with a starting point, and the ordering rules are the same.
func foldFrom(st *State, ops []Op) *State {
	if st == nil {
		st = NewState()
	}
	for _, op := range ops {
		if _, dup := st.OpIDs[op.ID]; dup {
			st.advanceSeq(op.Seq)
			st.Rejected++
			continue
		}
		st.applyReplayed(op)
	}
	return st
}

func (st *State) advanceSeq(seq uint64) {
	if seq > st.Seq {
		st.Seq = seq
	}
}

func (st *State) applyReplayed(op Op) {
	st.advanceSeq(op.Seq)
	if err := applyOp(st, op); err != nil {
		st.Rejected++
		return
	}
	if op.ID != "" {
		st.OpIDs[op.ID] = op.Seq
	}
	st.Applied++
}

// Snapshot is the read-only view callers get for a given instant.
type Snapshot struct {
	State *State    `json:"state"`
	Ready []string  `json:"ready"`
	Live  []string  `json:"live"`
	Stale []string  `json:"stale"`
	At    time.Time `json:"at"`
}

// Snapshot derives the predicates a scheduling read needs without storing them.
func (st *State) Snapshot(now time.Time) Snapshot {
	snap := Snapshot{State: st, At: now}
	for _, id := range st.sortedNodeIDs() {
		n := st.Nodes[id]
		if n.ClaimExpired(now) {
			snap.Live = append(snap.Live, id)
		}
		if n.Ready(st) {
			snap.Ready = append(snap.Ready, id)
		}
		if n.State == StateStale {
			snap.Stale = append(snap.Stale, id)
		}
	}
	return snap
}
