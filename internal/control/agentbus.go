package control

import (
	"context"
	"path/filepath"
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
	cursor      uint64
}

// SetAgentBusDir enrols this session on the board stored at dir. An empty dir
// opts out, and an opted-out controller composes exactly as it did before the
// feature existed.
func (c *Controller) SetAgentBusDir(dir string) {
	dir = strings.TrimSpace(dir)
	c.mu.Lock()
	defer c.mu.Unlock()
	if dir == "" {
		c.agentBus = nil
		return
	}
	c.agentBus = &agentBusState{dir: dir}
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
	return b.cursor
}

func (b *agentBusState) advance(seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if seq > b.cursor {
		b.cursor = seq
	}
}
