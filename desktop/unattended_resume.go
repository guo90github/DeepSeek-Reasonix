package main

import (
	"log/slog"
	"strings"
	"sync"

	"reasonix/internal/control"
)

// unattendedResumePrompt is what an interrupted unattended session is asked to carry on
// with: the user's own recipe — type 继续, press enter — applied by the host, because
// nothing else asks a restored session to keep working. The Goal, the board and the parked
// work all sit still until a turn arrives (found on a real machine, 2026-10-03).
const unattendedResumePrompt = "继续"

var (
	previousHostRunExitMu sync.Mutex
	previousHostRunKind   string

	unattendedResumeMu     sync.Mutex
	unattendedResumeQueued = map[string]bool{}
)

// notePreviousHostRunExit remembers how the run before this one ended, before this run's
// own marker overwrites the evidence. Only an unclean end means a turn was interrupted.
func notePreviousHostRunExit(verdict desktopExitVerdict) {
	previousHostRunExitMu.Lock()
	defer previousHostRunExitMu.Unlock()
	previousHostRunKind = verdict.Kind
}

func previousHostRunWasInterrupted() bool {
	previousHostRunExitMu.Lock()
	defer previousHostRunExitMu.Unlock()
	return previousHostRunKind != "" && previousHostRunKind != exitKindClean
}

// shouldResumeUnattendedSession decides whether this session gets the resume turn. The
// unattended switch is the only gate on this side of the decision (a human's intervention
// does not change that), and a session without a Goal has no contract to carry out.
func shouldResumeUnattendedSession(interrupted, unattended bool, goal string, alreadyQueued bool) bool {
	return interrupted && unattended && strings.TrimSpace(goal) != "" && !alreadyQueued
}

// resumeUnattendedSessionAfterAnInterruptedRun queues that turn once per host run per
// session and reports whether it did. The inbox keeps it durable, so a session that is
// still starting receives it as soon as it can take a turn.
func (a *App) resumeUnattendedSessionAfterAnInterruptedRun(tab *WorkspaceTab, ctrl control.SessionAPI) bool {
	if tab == nil || ctrl == nil {
		return false
	}
	sessionPath := ctrl.SessionPath()
	key := "unattended-resume|" + desktopRunID + "|" + sessionPath
	unattendedResumeMu.Lock()
	alreadyQueued := unattendedResumeQueued[key]
	unattendedResumeMu.Unlock()
	unattended := a.heartbeat != nil && a.heartbeat.unattendedEnabled()
	if !shouldResumeUnattendedSession(previousHostRunWasInterrupted(), unattended, tab.goal, alreadyQueued) {
		return false
	}
	unattendedResumeMu.Lock()
	unattendedResumeQueued[key] = true
	unattendedResumeMu.Unlock()
	if _, err := ctrl.TryEnqueueFollowup(control.InboxRequest{
		Submit:      unattendedResumePrompt,
		Raw:         unattendedResumePrompt,
		Source:      "unattended-resume",
		Idempotency: key,
	}); err != nil {
		slog.Warn("desktop: queue the unattended resume turn", "err", err, "session", sessionPath)
		return false
	}
	slog.Info("desktop: unattended session resumed after an interrupted host run", "session", sessionPath)
	return true
}
