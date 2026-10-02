package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

// AgentBusStatusView is the panel's first question: is this session on a board, which
// one, and where would joining put it. A session that never joined says so instead of
// being drawn as an empty board (AGENT_BUS §13.5).
type AgentBusStatusView struct {
	Enrolled    bool   `json:"enrolled"`
	Participant string `json:"participant"`
	Board       string `json:"board"`
	BoardDir    string `json:"boardDir"`
	DefaultDir  string `json:"defaultDir"`
}

// AgentBusStatus answers even when nothing is enrolled: "not on a board" is a state
// the panel draws an entry for, not an error it reports.
func (a *App) AgentBusStatus() (AgentBusStatusView, error) {
	bus, _ := a.agentBusControl()
	return agentBusStatusOf(bus, agentBusDefaultBoardDir()), nil
}

// AgentBusJoin puts the active session on the default board and hands the board its
// wake routing. Enrolling is what makes the board's work reachable at all: without it
// the session reads and writes nothing (AGENT_BUS §编排：会话是参与者).
func (a *App) AgentBusJoin() (AgentBusStatusView, error) {
	ctrl := a.activeCtrl()
	if ctrl == nil {
		return AgentBusStatusView{}, fmt.Errorf("desktop: no active session")
	}
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return AgentBusStatusView{}, fmt.Errorf("desktop: this session cannot join a board")
	}
	dir := agentBusDefaultBoardDir()
	if dir == "" {
		return AgentBusStatusView{}, fmt.Errorf("desktop: no state home to keep a board in")
	}
	bus.SetAgentBus(dir, "")
	// Remember the join: a host that restarts must not silently drop off the board, or
	// the board stops advancing the moment it was restored.
	if err := rememberAgentBusEnrolment(ctrl.SessionPath(), dir, bus.AgentBusParticipant()); err != nil {
		return AgentBusStatusView{}, fmt.Errorf("desktop: remember the board: %w", err)
	}
	a.enrollAgentBus(ctrl)
	return agentBusStatusOf(bus, dir), nil
}

// restoreAgentBusEnrolmentFor puts a session back on the board it joined before this host
// started, so a restored host keeps advancing the work it was part of.
//
// The caller must pass the session it really is. A controller learns its path only when
// the session is bound, so at build time SessionPath() is still empty: reading it there
// restored nothing at all, which is how the first cut of this fix missed on a real
// machine (2026-10-03).
func (a *App) restoreAgentBusEnrolmentFor(sessionPath string, ctrl control.SessionAPI) {
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok || bus.AgentBusEnrolled() {
		return
	}
	record, ok := rememberedAgentBusEnrolment(sessionPath)
	if !ok {
		return
	}
	bus.SetAgentBus(record.Dir, record.Participant)
	// The waker captured the board the controller had at build time, which was none:
	// re-arm it with the board this session just rejoined, exactly as a fresh join does.
	a.enrollAgentBus(ctrl)
}

// restoreAgentBusEnrolment is the build-time attempt: it only reaches a controller that
// already carries its session path.
func (a *App) restoreAgentBusEnrolment(ctrl control.SessionAPI) {
	a.restoreAgentBusEnrolmentFor(ctrl.SessionPath(), ctrl)
}

// AgentBusLeave opts the session out. The board and its log stay on disk untouched:
// leaving is losing sight of the work, never deleting it.
func (a *App) AgentBusLeave() (AgentBusStatusView, error) {
	bus, err := a.agentBusControl()
	if err != nil {
		return AgentBusStatusView{}, err
	}
	if err := forgetAgentBusEnrolment(ctrlSessionPath(bus)); err != nil {
		return AgentBusStatusView{}, fmt.Errorf("desktop: forget the board: %w", err)
	}
	bus.SetAgentBus("", "")
	return agentBusStatusOf(bus, agentBusDefaultBoardDir()), nil
}

// ctrlSessionPath reads the session path from the controller a host holds.
func ctrlSessionPath(bus control.AgentBusControl) string {
	session, ok := bus.(interface{ SessionPath() string })
	if !ok {
		return ""
	}
	return session.SessionPath()
}

func (a *App) agentBusControl() (control.AgentBusControl, error) {
	ctrl := a.activeCtrl()
	if ctrl == nil {
		return nil, fmt.Errorf("desktop: no active session")
	}
	bus, ok := ctrl.(control.AgentBusControl)
	if !ok {
		return nil, fmt.Errorf("desktop: this session cannot join a board")
	}
	return bus, nil
}

// agentBusStatusOf keeps the status shape in one place so join, leave and status
// cannot drift apart.
func agentBusStatusOf(bus control.AgentBusControl, defaultDir string) AgentBusStatusView {
	view := AgentBusStatusView{DefaultDir: defaultDir}
	if bus == nil {
		return view
	}
	view.Enrolled = bus.AgentBusEnrolled()
	if !view.Enrolled {
		return view
	}
	view.Participant = bus.AgentBusParticipant()
	if dir := strings.TrimSpace(bus.AgentBusDir()); dir != "" {
		view.BoardDir = dir
		view.Board = filepath.Base(dir)
	}
	return view
}

// agentBusDefaultBoardDir is the board a session joins when the user has not picked
// one: a single machine-local board under the state home (AGENT_BUS §5).
func agentBusDefaultBoardDir() string {
	root := strings.TrimSpace(config.UserSupportDir())
	if root == "" {
		return ""
	}
	return filepath.Join(root, "agentbus", "default")
}
