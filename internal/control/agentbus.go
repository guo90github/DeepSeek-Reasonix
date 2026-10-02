package control

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// agentBusState is this controller's participation in one coordination scope:
// where the board lives, who we are on it, and how far we have read. Keeping it
// in one sub-state leaves the controller's own scalar field count unchanged.
type agentBusState struct {
	mu          sync.Mutex
	dir         string
	participant string
	cursors     agentBusCursors
	limits      agentbus.TalkLimits
	// waker is the host's routing for ready-work wakes; nil means wake nobody.
	waker func(context.Context, agentbus.WakeTarget) error
	// woken remembers the last work set each participant was woken for, so a host
	// may re-run the sweep every tick without waking anyone twice.
	woken map[string]string
}

// agentBusCursors are this participant's "delivered up to" watermarks, one per
// surface. They share a lifetime, so they travel as one value rather than as
// independent fields.
type agentBusCursors struct {
	board uint64
	talk  uint64
}

// SetAgentBus enrols this session on the board stored at dir under an explicit
// participant id. An empty dir opts out; an empty id falls back to the session's
// branch id, and when the session has none either the participant sees nothing —
// a board identity is never guessed from a path.
func (c *Controller) SetAgentBus(dir, participant string) {
	dir = strings.TrimSpace(dir)
	participant = strings.TrimSpace(participant)
	c.mu.Lock()
	defer c.mu.Unlock()
	if dir == "" {
		c.agentBus = nil
		return
	}
	c.agentBus = &agentBusState{dir: dir, participant: participant}
}

// AgentBusView peeks at this participant's view without advancing the cursor.
// Frontends and diagnostics read it; the turn path consumes deltas instead.
func (c *Controller) AgentBusView(now time.Time) (agentbus.View, bool) {
	bus, st := c.agentBusSnapshot(now)
	if bus == nil {
		return agentbus.View{}, false
	}
	return agentbus.BuildView(st, agentbus.ViewSpec{
		Board:       filepath.Base(bus.dir),
		Participant: bus.participantID(c),
		Cursor:      bus.currentCursor(),
	}), true
}

// agentBusTurnBlock renders everything addressed to this participant since the
// last turn and advances the cursor. It rides the turn body like memory-update
// and background-jobs: never the cache-stable prefix (AGENT_BUS §8 R3).
func (c *Controller) agentBusTurnBlock() string {
	bus, st := c.agentBusSnapshot(time.Now().UTC())
	if bus == nil {
		return ""
	}
	view := agentbus.BuildView(st, agentbus.ViewSpec{
		Board:       filepath.Base(bus.dir),
		Participant: bus.participantID(c),
		Cursor:      bus.currentCursor(),
	})
	if len(view.Lines) == 0 {
		return ""
	}
	bus.advance(view.Next)
	return view.Render()
}

// agentBusSnapshot folds the board for this participant. A board that cannot be
// read contributes nothing: an unreachable view must never break a turn.
// errAgentBusUnwired reports a write attempted by a session that never joined a
// board; the kernel refuses instead of inventing a scope.
var errAgentBusUnwired = errors.New("control: this session is not enrolled on a board")

// ApplyAgentBusOp writes one op to the board this session is enrolled on. The
// board validates it and a refusal comes back typed (board.IsReject), so a caller
// reports why rather than guessing.
func (c *Controller) ApplyAgentBusOp(ctx context.Context, op board.Op) (board.Receipt, error) {
	c.mu.Lock()
	bus := c.agentBus
	c.mu.Unlock()
	if bus == nil {
		return board.Receipt{}, errAgentBusUnwired
	}
	brd, err := board.Open(bus.dir)
	if err != nil {
		return board.Receipt{}, err
	}
	receipt, err := brd.Apply(ctx, op)
	if err != nil {
		return receipt, err
	}
	// The writer that makes work startable wakes whoever asked for it rather than
	// waiting for a tick: work that can begin now should begin now (AGENT_BUS §13.2).
	// A replayed write adds nothing, so it wakes nobody.
	if !receipt.Duplicate {
		c.WakeAgentBus(ctx)
	}
	return receipt, nil
}

// AgentBusTask is one board node as the human side renders it: flat rows the task
// tree nests by Deps, so the panel needs no new storage.
type AgentBusTask struct {
	ID         string
	Title      string
	State      board.NodeState
	Owner      string
	Deps       []string
	Ready      bool
	Refuted    bool
	NoProgress int
	LastSeq    uint64
}

// AgentBusTasks projects the whole board into human rows, sorted by id so two
// reads of the same board agree.
func (c *Controller) AgentBusTasks(now time.Time) ([]AgentBusTask, bool) {
	bus, st := c.agentBusSnapshot(now)
	if bus == nil {
		return nil, false
	}
	ids := make([]string, 0, len(st.Nodes))
	for id := range st.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]AgentBusTask, 0, len(ids))
	for _, id := range ids {
		n := st.Nodes[id]
		out = append(out, AgentBusTask{
			ID: n.ID, Title: n.Title, State: n.State, Owner: n.Owner,
			Deps: append([]string(nil), n.Deps...), Ready: n.Ready(st),
			Refuted: len(n.Refutes) > 0, NoProgress: n.NoProgress, LastSeq: n.LastSeq,
		})
	}
	return out, true
}

func (c *Controller) agentBusSnapshot(now time.Time) (*agentBusState, *board.State) {
	c.mu.Lock()
	bus := c.agentBus
	c.mu.Unlock()
	if bus == nil {
		return nil, nil
	}
	brd, err := board.Open(bus.dir)
	if err != nil {
		return nil, nil
	}
	st, err := brd.Snapshot(context.Background(), now)
	if err != nil {
		return nil, nil
	}
	return bus, st
}

// participantID resolves our identity on the board on first use, because a
// controller learns its session path after it is constructed.
func (b *agentBusState) participantID(c *Controller) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.participant == "" {
		b.participant = c.parentSessionID()
	}
	return b.participant
}

func (b *agentBusState) currentCursor() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cursors.board
}

func (b *agentBusState) advance(seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if seq > b.cursors.board {
		b.cursors.board = seq
	}
}
