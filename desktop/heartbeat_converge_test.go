package main

import (
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/sessioninbox"
)

type heartbeatGoalCtrlStub struct {
	goal     string
	status   string
	anchored string
	resumed  bool
	refuse   bool
	planMode bool
	paused   bool
	unpaused bool
}

func (s *heartbeatGoalCtrlStub) RuntimeStatus() control.RuntimeStatus {
	return control.RuntimeStatus{}
}

func (s *heartbeatGoalCtrlStub) Goal() string { return s.goal }

func (s *heartbeatGoalCtrlStub) GoalStatus() string { return s.status }

func (s *heartbeatGoalCtrlStub) SetGoal(goal string) {
	s.anchored = goal
	s.goal = goal
	s.status = control.GoalStatusRunning
}

func (s *heartbeatGoalCtrlStub) ResumeGoal() bool {
	if s.refuse {
		return false
	}
	s.resumed = true
	s.status = control.GoalStatusRunning
	return true
}

func (s *heartbeatGoalCtrlStub) PlanMode() bool { return s.planMode }

func (s *heartbeatGoalCtrlStub) SetPlanMode(v bool) { s.planMode = v }

func (s *heartbeatGoalCtrlStub) InboxSnapshot() sessioninbox.InboxSnapshot {
	return sessioninbox.InboxSnapshot{Paused: s.paused}
}

func (s *heartbeatGoalCtrlStub) SetInboxPaused(paused bool) error {
	s.paused = paused
	s.unpaused = !paused
	return nil
}

func TestHeartbeatUnattendedStepClearsTheGatesThatNeedAHuman(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "ship", status: control.GoalStatusRunning, planMode: true, paused: true}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	engine.unattendedGoalStepWith(&task, stub, true)
	if stub.planMode {
		t.Fatal("plan mode must be off: nobody is there to answer its approval gate")
	}
	if !stub.unpaused || stub.paused {
		t.Fatalf("a paused inbox must be resumed: %+v", stub)
	}
}

func TestHeartbeatUnattendedStepLeavesTheGatesAloneWhileTheSwitchIsOff(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "ship", status: control.GoalStatusRunning, planMode: true, paused: true}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	engine.unattendedGoalStepWith(&task, stub, false)
	if !stub.planMode || !stub.paused || stub.unpaused {
		t.Fatalf("the switch is off: the session's gates are the human's business: %+v", stub)
	}
}

func TestHeartbeatGoalDecideWithoutAContractKeepsThePlainPath(t *testing.T) {
	for _, task := range []HeartbeatTask{{}, {Goal: "   "}} {
		action, reason := heartbeatGoalDecide(task, "an unrelated goal", control.GoalStatusBlocked)
		if action != heartbeatGoalSubmit || reason != "" {
			t.Fatalf("plain task decided (%v, %q), want submit with no reason", action, reason)
		}
	}
}

func TestHeartbeatGoalDecideAnchorsReplacesAndHolds(t *testing.T) {
	task := HeartbeatTask{Goal: "ship the unattended driver"}
	cases := []struct {
		name   string
		goal   string
		status string
		want   heartbeatGoalAction
	}{
		{"no Goal anchored yet", "", "", heartbeatGoalAnchor},
		{"a human's Goal is replaced by the contract", "what the human typed", control.GoalStatusRunning, heartbeatGoalAnchor},
		{"own Goal is running", task.Goal, control.GoalStatusRunning, heartbeatGoalHold},
		{"own Goal is complete", task.Goal, control.GoalStatusComplete, heartbeatGoalHold},
		{"unknown status", task.Goal, "surprising", heartbeatGoalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, _ := heartbeatGoalDecide(task, tc.goal, tc.status)
			if action != tc.want {
				t.Fatalf("decided %v, want %v", action, tc.want)
			}
		})
	}
}

func TestHeartbeatGoalDecideResumesEveryRecoverableStop(t *testing.T) {
	task := HeartbeatTask{Goal: "ship the unattended driver"}
	for _, status := range []string{control.GoalStatusBlocked, control.GoalStatusStopped} {
		if action, _ := heartbeatGoalDecide(task, task.Goal, status); action != heartbeatGoalResume {
			t.Fatalf("%s decided %v, want resume: the switch means on, with no retry ceiling", status, action)
		}
	}
}

func TestHeartbeatUnattendedSwitchIsSnapshottedNotLive(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	if engine.unattendedEnabled() {
		t.Fatal("a fresh engine is off until Start reads the config")
	}
	engine.unattended = true
	if !engine.unattendedEnabled() {
		t.Fatal("the driver must read Start's snapshot, so a live toggle cannot change this process")
	}
}

func TestHeartbeatUnattendedStepIsInertWhileTheSwitchIsOff(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "what the human typed"}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	if engine.unattendedGoalStepWith(&task, stub, false) {
		t.Fatal("driver stopped the tick while the master switch was off")
	}
	if stub.anchored != "" || stub.resumed || task.LastRunAt != 0 || stub.goal != "what the human typed" {
		t.Fatalf("driver touched the session with the switch off: %+v lastRunAt=%d", stub, task.LastRunAt)
	}
}

func TestHeartbeatUnattendedStepAnchorsThenSubmits(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	if engine.unattendedGoalStepWith(&task, stub, true) {
		t.Fatal("anchoring must let this tick submit its prompt")
	}
	if stub.anchored != "ship" || stub.goal != "ship" {
		t.Fatalf("anchored = %q, goal = %q, want the task contract", stub.anchored, stub.goal)
	}
}

func TestHeartbeatUnattendedStepReplacesAHumanGoalAndKeepsGoing(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "what the human typed", status: control.GoalStatusRunning}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	if engine.unattendedGoalStepWith(&task, stub, true) {
		t.Fatal("an intervention must not stop the task while the switch is on")
	}
	if stub.goal != "ship" {
		t.Fatalf("goal = %q, want the task contract restored", stub.goal)
	}
	if task.LastRunAt != 0 {
		t.Fatal("a re-anchored tick must go on to submit its prompt")
	}
}

func TestHeartbeatUnattendedStepHoldsARunningGoal(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "ship", status: control.GoalStatusRunning}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	if !engine.unattendedGoalStepWith(&task, stub, true) {
		t.Fatal("a running Goal must hold the tick: its own continuation owns the next turn")
	}
	if task.LastRunAt == 0 {
		t.Fatal("a held tick must consume the interval, or it repeats every 30s")
	}
	if stub.resumed || stub.anchored != "" {
		t.Fatalf("holding modified the Goal: %+v", stub)
	}
}

func TestHeartbeatUnattendedStepResumesBlockedWithoutACeiling(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "ship", status: control.GoalStatusBlocked}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	for round := 0; round < 10; round++ {
		if engine.unattendedGoalStepWith(&task, stub, true) {
			t.Fatalf("resume %d stopped the tick instead of continuing the work", round+1)
		}
		if !stub.resumed {
			t.Fatalf("resume %d did not call ResumeGoal", round+1)
		}
		stub.resumed = false
		stub.status = control.GoalStatusBlocked
	}
	if task.LastRunAt != 0 {
		t.Fatal("a resumed tick must go on to submit its prompt")
	}
}

func TestHeartbeatUnattendedStepHoldsAndConsumesWhenResumeIsRefused(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	stub := &heartbeatGoalCtrlStub{goal: "ship", status: control.GoalStatusBlocked, refuse: true}
	task := HeartbeatTask{ID: "t", Goal: "ship"}
	if !engine.unattendedGoalStepWith(&task, stub, true) {
		t.Fatal("a refused resume must not fall through to submitting a prompt")
	}
	if task.LastRunAt == 0 {
		t.Fatal("a refused resume must consume the interval instead of retrying every 30s")
	}
}
