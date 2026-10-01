// Wedged-turn policy — an unattended run must not be stopped forever by a turn
// that stopped producing events. The kernel reports the silence; the driver
// decides, because only unattended driving may trade a long tool for progress.

package main

import (
	"log"
	"time"

	"reasonix/internal/control"
)

// heartbeatTurnSilence is the kernel's read-only observation of a running turn.
type heartbeatTurnSilence interface {
	TurnSilence(now time.Time) (time.Duration, bool)
}

// heartbeatTurnCanceller is the old, battle-tested cancel path the UI's Stop
// button uses, so a cancelled turn ends exactly as a manual stop would.
type heartbeatTurnCanceller interface {
	Cancel()
}

// cancelWedgedUnattendedTurn cancels a turn that has been silent longer than the
// bound the kernel already calls stuck, so the next tick can hand the task off
// instead of skipping forever. Attended sessions are untouched: there, a long
// tool and a wedged one look identical.
func (e *HeartbeatEngine) cancelWedgedUnattendedTurn(t HeartbeatTask, ctrl heartbeatRuntimeStatus) {
	if ctrl == nil || !e.unattendedEnabled() {
		return
	}
	silence, ok := heartbeatTurnSilenceOf(ctrl, time.Now())
	if !ok || silence < control.TurnStallThreshold() {
		return
	}
	canceller, ok := ctrl.(heartbeatTurnCanceller)
	if !ok || !e.noteGoalHold(t.ID, "a turn with no progress was cancelled") {
		return
	}
	canceller.Cancel()
	log.Printf("[heartbeat] unattended %q cancelled a turn with no progress for %s; the next tick resumes the task", t.Title, silence.Round(time.Minute))
}

// heartbeatTurnSilenceOf reads the silence from a controller that reports it.
func heartbeatTurnSilenceOf(ctrl heartbeatRuntimeStatus, now time.Time) (time.Duration, bool) {
	silence, ok := ctrl.(heartbeatTurnSilence)
	if !ok {
		return 0, false
	}
	return silence.TurnSilence(now)
}
