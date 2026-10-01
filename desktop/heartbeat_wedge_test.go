package main

import (
	"testing"
	"time"

	"reasonix/internal/control"
)

// heartbeatWedgeStub is a controller that reports its silence and counts the
// cancellations the driver asks for.
type heartbeatWedgeStub struct {
	silence time.Duration
	running bool
	cancels int
}

func (s *heartbeatWedgeStub) RuntimeStatus() control.RuntimeStatus { return control.RuntimeStatus{} }

func (s *heartbeatWedgeStub) TurnSilence(time.Time) (time.Duration, bool) {
	return s.silence, s.running
}

func (s *heartbeatWedgeStub) Cancel() { s.cancels++ }

func TestWedgedTurnIsCancelledOnlyWhileUnattended(t *testing.T) {
	stalled := func() *heartbeatWedgeStub {
		return &heartbeatWedgeStub{silence: control.TurnStallThreshold() + time.Minute, running: true}
	}

	engine := newHeartbeatEngine(nil)
	engine.unattended = true
	task := HeartbeatTask{ID: "t1", Title: "task"}

	ctrl := stalled()
	engine.cancelWedgedUnattendedTurn(task, ctrl)
	if ctrl.cancels != 1 {
		t.Fatalf("cancels = %d, want the wedged turn cancelled once", ctrl.cancels)
	}
	engine.cancelWedgedUnattendedTurn(task, ctrl)
	if ctrl.cancels != 1 {
		t.Fatalf("cancels = %d, want no repeat cancellation for the same silence", ctrl.cancels)
	}

	engine.unattended = false
	attended := stalled()
	engine.cancelWedgedUnattendedTurn(task, attended)
	if attended.cancels != 0 {
		t.Fatal("an attended session must never have its turn cancelled by the driver")
	}
}

func TestAHealthyTurnIsNeverCancelled(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	engine.unattended = true
	task := HeartbeatTask{ID: "t1", Title: "task"}

	for _, tc := range []struct {
		name string
		ctrl heartbeatRuntimeStatus
	}{
		{"a turn still producing events", &heartbeatWedgeStub{silence: time.Minute, running: true}},
		{"an idle controller", &heartbeatWedgeStub{silence: control.TurnStallThreshold() * 2}},
		{"a controller that reports no liveness", &heartbeatGoalCtrlStub{}},
		{"no controller", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine.cancelWedgedUnattendedTurn(task, tc.ctrl)
			if stub, ok := tc.ctrl.(*heartbeatWedgeStub); ok && stub.cancels != 0 {
				t.Fatalf("cancels = %d, want none", stub.cancels)
			}
		})
	}
}
