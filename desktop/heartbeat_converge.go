// Unattended driver — the Goal half of a heartbeat task: a task with a Goal
// contract keeps one Goal alive across ticks (anchor, resume, hold). The config
// master switch is the only gate, and it is read once at Start.

package main

import (
	"log"
	"strings"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/sessioninbox"
)

// heartbeatGoalRuntime is the Goal half of the controller port the driver
// needs. Kept apart from heartbeatRuntimeStatus so a controller without Goal
// support leaves the driver inert instead of failing to compile.
type heartbeatGoalRuntime interface {
	Goal() string
	GoalStatus() string
	SetGoal(goal string)
	ResumeGoal() bool
}

// heartbeatSessionGuards is the pair of gates that stop a long task with no one
// to answer them: plan mode's approval gate and a paused inbox.
type heartbeatSessionGuards interface {
	PlanMode() bool
	SetPlanMode(v bool)
	InboxSnapshot() sessioninbox.InboxSnapshot
	SetInboxPaused(paused bool) error
}

type heartbeatGoalAction int

const (
	heartbeatGoalSubmit heartbeatGoalAction = iota // no Goal: submit the prompt
	heartbeatGoalAnchor                            // (re)install the task contract as the session's Goal
	heartbeatGoalResume                            // recoverable stop: resume it
	heartbeatGoalHold                              // do not submit this tick
)

// heartbeatGoalDecide is the pure decision: the task's contract against the
// session's Goal state. It never touches the controller.
func heartbeatGoalDecide(task HeartbeatTask, goal, status string) (heartbeatGoalAction, string) {
	contract := strings.TrimSpace(task.Goal)
	if contract == "" {
		return heartbeatGoalSubmit, ""
	}
	current := strings.TrimSpace(goal)
	if current == "" {
		return heartbeatGoalAnchor, "no Goal is anchored yet"
	}
	if current != contract {
		// The switch is the only gate, so anything a human set here is replaced
		// rather than obeyed: an intervention never stops the task.
		return heartbeatGoalAnchor, "re-anchoring the task contract over the session's Goal"
	}
	switch status {
	case control.GoalStatusRunning:
		return heartbeatGoalHold, "the Goal is running; its own continuation drives the next turn"
	case control.GoalStatusComplete:
		return heartbeatGoalHold, "the Goal is complete; the driver stops submitting"
	case control.GoalStatusBlocked, control.GoalStatusStopped:
		// No retry ceiling: the switch means on, so a recoverable stop is
		// resumed every interval for as long as the switch stays on.
		return heartbeatGoalResume, "recoverable stop; resuming the Goal"
	default:
		return heartbeatGoalHold, "unrecognised Goal status " + status
	}
}

// unattendedEnabled reports the master switch snapshotted at Start, so toggling
// it never changes what the running process does.
func (e *HeartbeatEngine) unattendedEnabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.unattended
}

// unattendedGoalStep applies one driver step for this tick and reports whether
// the caller must stop before submitting; it is a no-op with the switch off or
// on a controller that cannot hold a Goal.
func (e *HeartbeatEngine) unattendedGoalStep(t *HeartbeatTask, ctrl heartbeatRuntimeStatus) bool {
	return e.unattendedGoalStepWith(t, ctrl, e.unattendedEnabled())
}

// unattendedGoalStepWith is the switch-injected form, so the decision can be
// tested without a config file on disk. The switch is the gate, never the Goal: a
// task without a contract still gets the unattended care — it just has no Goal to
// steer.
func (e *HeartbeatEngine) unattendedGoalStepWith(t *HeartbeatTask, ctrl heartbeatRuntimeStatus, on bool) bool {
	if t == nil || !on {
		return false
	}
	e.clearUnattendedGates(t, ctrl)
	if strings.TrimSpace(t.Goal) == "" {
		return false
	}
	goals, ok := ctrl.(heartbeatGoalRuntime)
	if !ok {
		return false
	}
	action, reason := heartbeatGoalDecide(*t, goals.Goal(), goals.GoalStatus())
	switch action {
	case heartbeatGoalAnchor:
		goals.SetGoal(strings.TrimSpace(t.Goal))
	case heartbeatGoalResume:
		if !goals.ResumeGoal() {
			return e.holdTick(t, reason)
		}
	case heartbeatGoalHold:
		return e.holdTick(t, reason)
	}
	return false
}

// holdTick consumes this tick so a steady state is re-checked on the next
// interval instead of on every 30s scheduler tick.
func (e *HeartbeatEngine) holdTick(t *HeartbeatTask, reason string) bool {
	// A hold is not a run: the schedule counts from it, LastRunAt does not.
	t.LastAttemptAt = time.Now().UnixMilli()
	if e.noteGoalHold(t.ID, reason) {
		log.Printf("[heartbeat] unattended %q holds: %s", t.Title, reason)
	}
	return true
}

// noteGoalHold records a hold reason and reports whether it changed, so a
// steady state logs once instead of every interval.
func (e *HeartbeatEngine) noteGoalHold(taskID, reason string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.holdLog == nil {
		e.holdLog = make(map[string]string)
	}
	if e.holdLog[taskID] == reason {
		return false
	}
	e.holdLog[taskID] = reason
	return true
}

// clearUnattendedGates removes the two stops that only a human could answer:
// plan mode (its approval gate would wait forever) and a paused inbox (inbox
// recovery pauses it until someone looks).
func (e *HeartbeatEngine) clearUnattendedGates(t *HeartbeatTask, ctrl heartbeatRuntimeStatus) {
	guards, ok := ctrl.(heartbeatSessionGuards)
	if !ok {
		return
	}
	if guards.PlanMode() {
		guards.SetPlanMode(false)
		log.Printf("[heartbeat] unattended %q turned plan mode off", t.Title)
	}
	if guards.InboxSnapshot().Paused {
		if err := guards.SetInboxPaused(false); err != nil {
			log.Printf("[heartbeat] unattended %q could not unpause the inbox: %v", t.Title, err)
			return
		}
		log.Printf("[heartbeat] unattended %q resumed the paused inbox", t.Title)
	}
}
