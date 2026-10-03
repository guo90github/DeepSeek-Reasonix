package main

import (
	"fmt"
	"os"
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
	// Joining is the one switch: it also lifts a host-level opt-out.
	if err := setAgentBusOptOut(false); err != nil {
		return AgentBusStatusView{}, fmt.Errorf("desktop: clear the opt-out: %w", err)
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
	record, remembered := rememberedAgentBusEnrolment(sessionPath)
	if !remembered {
		// No record for this session, but a board this machine already uses: joining is a
		// choice the user made for the host, and a new session silently left off the board
		// reads nothing and is told nothing (found on a real machine, 2026-10-03).
		dir := agentBusUsedBoardDir()
		if dir == "" {
			return
		}
		record = agentBusEnrolment{Dir: dir}
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

// agentBusUsedBoardDir names the default board when this machine has one on disk and the
// user has not opted out, and nothing when it does not: a host that never joined stays out.
func agentBusUsedBoardDir() string {
	if agentBusOptedOut() {
		return ""
	}
	dir := agentBusDefaultBoardDir()
	if dir == "" {
		return ""
	}
	info, err := os.Stat(filepath.Join(dir, "board.jsonl"))
	if err != nil || info.IsDir() {
		return ""
	}
	return dir
}

// AgentBusLeave opts the session out. The board and its log stay on disk untouched:
// leaving is losing sight of the work, never deleting it.
//
// It also records the opt-out for the host, because a session that simply forgets its board
// would be put straight back on it by the machine-level fallback above.
func (a *App) AgentBusLeave() (AgentBusStatusView, error) {
	bus, err := a.agentBusControl()
	if err != nil {
		return AgentBusStatusView{}, err
	}
	if err := forgetAgentBusEnrolment(ctrlSessionPath(bus)); err != nil {
		return AgentBusStatusView{}, fmt.Errorf("desktop: forget the board: %w", err)
	}
	if err := setAgentBusOptOut(true); err != nil {
		return AgentBusStatusView{}, fmt.Errorf("desktop: record the opt-out: %w", err)
	}
	bus.SetAgentBus("", "")
	return agentBusStatusOf(bus, agentBusDefaultBoardDir()), nil
}

// agentBusOptOutPath is the host-level "keep my sessions off the board" marker. Joining
// clears it, so the choice stays a single switch rather than a per-session accident.
func agentBusOptOutPath() string {
	root := strings.TrimSpace(config.UserSupportDir())
	if root == "" {
		return ""
	}
	return filepath.Join(root, "agentbus", "opt-out")
}

func agentBusOptedOut() bool {
	path := agentBusOptOutPath()
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func setAgentBusOptOut(off bool) error {
	path := agentBusOptOutPath()
	if path == "" {
		return nil
	}
	if !off {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("left the board; delete this file to rejoin on the next session\n"), 0o644)
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
