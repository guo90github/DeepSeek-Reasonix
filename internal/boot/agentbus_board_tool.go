package boot

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
)

// errBoardToolNotReady is what a board tool call gets in the window between the tool
// registry being assembled and control.New producing the controller it forwards to.
var errBoardToolNotReady = errors.New("boot: the session is not ready to talk to a board yet")

// errBoardToolUnreadable is a board that exists but could not be folded: distinct from
// "not ready" and from "joined nothing", because the caller's next move differs.
var errBoardToolUnreadable = errors.New("boot: the board could not be read")

// boardToolPort is the lazily-resolved binding behind the agent_bus tool: the tool
// registry exists before the controller does, so the port holds the same
// atomic.Pointer[control.Controller] the rest of the build does. Resolution happens per
// call, which is also what makes enrolment mid-session visible to the tool.
type boardToolPort struct {
	ctrl *atomic.Pointer[control.Controller]
}

func (p boardToolPort) controller() (*control.Controller, error) {
	if p.ctrl == nil {
		return nil, errBoardToolNotReady
	}
	c := p.ctrl.Load()
	if c == nil {
		return nil, errBoardToolNotReady
	}
	return c, nil
}

func (p boardToolPort) ApplyBoardOp(ctx context.Context, op board.Op) (board.Receipt, error) {
	c, err := p.controller()
	if err != nil {
		return board.Receipt{}, err
	}
	return c.ApplyAgentBusOp(ctx, op)
}

func (p boardToolPort) BoardView(now time.Time) (agentbus.View, error) {
	c, err := p.controller()
	if err != nil {
		return agentbus.View{}, err
	}
	view, ok := c.AgentBusView(now)
	if !ok {
		return agentbus.View{}, errBoardToolUnreadable
	}
	return view, nil
}

// BoardIdentity reports an empty directory for a session that joined nothing, which is
// the tool's cue to tell the caller what the user can do about it.
func (p boardToolPort) BoardIdentity() (string, string, error) {
	c, err := p.controller()
	if err != nil {
		return "", "", err
	}
	return c.AgentBusParticipant(), c.AgentBusDir(), nil
}
