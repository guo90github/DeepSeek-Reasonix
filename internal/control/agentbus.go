package control

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agent"
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
	// hearingLimits bound this session's deliberations; zero leaves them off.
	hearingLimits agentbus.HearingLimits
	// nodeRate bounds how often one node may move; zero leaves the ceiling off.
	nodeRate board.Limits
	// observeLimits tune how much the human first screen carries; zero uses defaults.
	observeLimits agentbus.ObserveLimits
	// waker is the host's routing for ready-work wakes; nil means wake nobody.
	waker func(context.Context, agentbus.WakeTarget) error
	// wakeLedger remembers the last work set each participant was woken for. A host
	// may share one across the controllers it rebuilds, so a rebuild cannot re-wake.
	wakeLedger *WakeLedger
	// ledger is the host's spending account; nil means nothing is charged at all.
	// Ceilings belong to the machine, so a host shares one account across controllers.
	ledger *agentbus.Ledger
	// dispatchRefusals remembers steps a ceiling refused, so the dispatch loop stops offering
	// them: re-parking a refused step every tick turned one refusal into a per-tick log pair.
	dispatchRefusals *dispatchRefusalMemo
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
	c.agentBus = &agentBusState{dir: dir, participant: participant, wakeLedger: NewWakeLedger()}
}

// AgentBusEnrolled reports whether a board was set for this session. A session can be
// enrolled before it has an identity on that board — its path may not exist yet — so
// this answers "was a board set", not "can it be read": the read methods, which need
// the identity, stay empty until it resolves.
func (c *Controller) AgentBusEnrolled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentBus != nil
}

// AgentBusAnnounce publishes where this session speaks from, so another host can wake
// it. A host that serves its own sessions passes its base URL and token file; a host
// with no endpoint (the desktop before it runs a serve) leaves it unannounced, and a
// wake addressed here is then refused rather than dropped silently.
func (c *Controller) AgentBusAnnounce(host, tokenFile string) error {
	state, err := c.agentBusForTalk()
	if err != nil {
		return err
	}
	directory, err := agentbus.OpenParticipantDirectory(state.dir)
	if err != nil {
		return err
	}
	_, err = directory.Announce(context.Background(), agentbus.ParticipantRef{
		Participant: state.participantID(c),
		Host:        strings.TrimSpace(host),
		SessionPath: c.SessionPath(),
		TokenFile:   strings.TrimSpace(tokenFile),
		At:          time.Now().UTC(),
	})
	return err
}

// AgentBusWithdraw retires this session's address: a session that left the board must
// not keep being woken somewhere it no longer is.
func (c *Controller) AgentBusWithdraw() error {
	state, err := c.agentBusForTalk()
	if err != nil {
		return err
	}
	directory, err := agentbus.OpenParticipantDirectory(state.dir)
	if err != nil {
		return err
	}
	return directory.Withdraw(context.Background(), state.participantID(c))
}

// AgentBusView peeks at this participant's view without advancing the cursor.
// Frontends and diagnostics read it; the turn path consumes deltas instead. A turn
// that already carried the delta leaves nothing new to show, so a participant that
// still has work gets the current set rather than a page it cannot re-read.
func (c *Controller) AgentBusView(now time.Time) (agentbus.View, bool) {
	bus, st := c.agentBusSnapshot(now)
	if bus == nil {
		return agentbus.View{}, false
	}
	spec := agentbus.ViewSpec{
		Board:       filepath.Base(bus.dir),
		Participant: bus.participantID(c),
		Cursor:      bus.currentCursor(),
	}
	view := agentbus.BuildView(st, spec)
	if len(view.Lines) == 0 && view.Owned+view.Waiting+view.Needed > 0 {
		spec.Cursor = 0
		view = agentbus.BuildView(st, spec)
	}
	return view, true
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
	brd, err := board.Open(bus.dir, bus.nodeRateLimits())
	if err != nil {
		return board.Receipt{}, err
	}
	// A lapsed lease has nobody left to notice it: the holder is gone and a claimed
	// node is invisible to the wake surface, so this write is the only thing that
	// runs. Reclaim before writing, so the wake below can advertise what it freed.
	if _, err := brd.Sweep(ctx, time.Now().UTC()); err != nil {
		slog.Warn("controller: agentbus reclaim before write", "board", bus.dir, "err", err)
	}
	// Work is paid for as it starts: an over-budget claim is refused before it lands.
	if err := bus.chargeClaim(op); err != nil {
		return board.Receipt{}, err
	}
	receipt, err := brd.Apply(ctx, op)
	if err != nil {
		// A rate refusal moves nothing and leaves no record, so the host's own count is the
		// only sign the ceiling is biting (G3).
		if reason, isReject := board.IsReject(err); isReject && reason == board.ReasonRateLimited {
			noteNodeRateRefusal(op.Node)
		}
		return receipt, err
	}
	// An accepted step settles against the board and subtree allowances — the levels
	// only accepted work may spend.
	if !receipt.Duplicate && op.Verb == board.VerbDecide && op.Outcome == board.OutcomeDone {
		if state, err := brd.Snapshot(ctx, time.Now().UTC()); err == nil {
			bus.settleBudget(state, op.Node)
		}
	}
	// The writer that makes work startable wakes whoever asked for it rather than
	// waiting for a tick: work that can begin now should begin now (AGENT_BUS §13.2).
	// A replayed write adds nothing, so it wakes nobody.
	if !receipt.Duplicate {
		c.WakeAgentBus(ctx)
	}
	return receipt, nil
}

// AgentBusBoardRows used to live here: a whole-board projection no frontend consumed (the
// panel reads Briefing/Detail, the tool reads a participant-scoped view). Removed rather than
// left looking used (G8); re-add with the §13.1 row cap if a frontend ever needs it.

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

// parentSessionID is the identity a session falls back to when no participant was
// given explicitly: the branch id derived from its session path. It lives here with
// the rest of the board identity so the controller struct stays as it was.
func (c *Controller) parentSessionID() string {
	return agent.BranchID(c.SessionPath())
}
