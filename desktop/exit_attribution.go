package main

import (
	"log"
)

// Exit attribution — the answer to "why is the desktop gone, again". Every launch
// classifies the run before it from the two records that survive a run: the note
// the process wrote about itself and the observation the watcher wrote about it.
// The verdict is local and unconditional, because the person asking is the person
// who was using the app, not a telemetry pipeline.
type desktopExitVerdict struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
	RunID  string `json:"runId,omitempty"`
	PID    int    `json:"pid,omitempty"`
	// At is when that run ended, when a record says. An empty At with a running
	// kind means nobody wrote an end: that is what a machine going down looks like.
	At string `json:"at,omitempty"`
	// ExitCode is the OS's code in hex, empty when the platform cannot report one.
	ExitCode string `json:"exitCode,omitempty"`
	// UncleanStreak is filled by the host-state marker, not by the classification.
	UncleanStreak int `json:"uncleanStreak,omitempty"`
}

// previousRunExit is this launch's verdict, carried into the host-state marker by
// noteHostLaunch so a reader does not have to re-derive it from four files.
var previousRunExit *desktopExitVerdict

// recordPreviousRunExit classifies the run before this one and states it in words
// in the durable log. It returns the verdict for callers that want it in hand.
func recordPreviousRunExit() desktopExitVerdict {
	note, noteOK := latestDesktopExitNote(desktopRunID)
	observation, observationOK := desktopExitObservation{}, false
	if noteOK {
		observation, observationOK = latestDesktopExitObservationFor(note.RunID, note.PID)
	}
	if !observationOK {
		observation, observationOK = lastDesktopExitObservation()
	}
	kind, reason := classifyDesktopRun(note, noteOK, observation, observationOK)
	verdict := desktopExitVerdict{Kind: kind, Reason: reason, RunID: note.RunID, PID: note.PID}
	if noteOK {
		verdict.At = note.ExitedAt
	}
	if observationOK {
		verdict.PID = observation.PID
		if observation.CodeKnown {
			verdict.ExitCode = desktopExitCodeText(observation.ExitCode)
		}
		if verdict.At == "" {
			verdict.At = observation.ObservedAt
		}
	}
	if noteOK || observationOK {
		log.Printf("[desktop] previous run ended as %s: %s (runId=%q pid=%d code=%s started=%s)",
			verdict.Kind, verdict.Reason, verdict.RunID, verdict.PID, verdict.ExitCode, note.StartedAt)
	}
	previousRunExit = &verdict
	return verdict
}

// lastDesktopExitObservation returns the newest observation in the store, whatever
// run it belongs to: a kill can land before the killed process writes its note at
// all, so the observation is sometimes the only record of the run.
func lastDesktopExitObservation() (desktopExitObservation, bool) {
	observations := readDesktopExitObservations()
	if len(observations) == 0 {
		return desktopExitObservation{}, false
	}
	newest := observations[0]
	for _, observation := range observations[1:] {
		if observation.ObservedAt >= newest.ObservedAt {
			newest = observation
		}
	}
	return newest, true
}
