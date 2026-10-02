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

// unattendedGoalContract is the Goal the user gave this session, taken from the tab or,
// when the tab's own copy is empty, from the persisted tab file. A crashed host leaves the
// goal's status stopped, and the running-goal accessor reports a stopped goal as no goal —
// which is how an unattended session ended up with nothing to resume (2026-10-03).
func unattendedGoalContract(tab *WorkspaceTab, sessionPath string) string {
	if tab != nil {
		if goal := strings.TrimSpace(tab.goal); goal != "" {
			return goal
		}
	}
	key := sessionRuntimeKey(sessionPath)
	if key == "" {
		return ""
	}
	for _, entry := range loadTabsFile().Tabs {
		if sessionRuntimeKey(strings.TrimSpace(entry.SessionPath)) == key {
			return strings.TrimSpace(entry.Goal)
		}
	}
	return ""
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
	interrupted := previousHostRunWasInterrupted()
	unattended := a.heartbeat != nil && a.heartbeat.unattendedEnabled()
	contract := unattendedGoalContract(tab, sessionPath)
	if !shouldResumeUnattendedSession(interrupted, unattended, contract, alreadyQueued) {
		// Say why: a session that never resumes otherwise looks exactly like one with
		// nothing to do, and this decision is the whole feature (2026-10-03).
		if unattended && !alreadyQueued {
			slog.Info("desktop: unattended resume declined", "session", sessionPath,
				"interrupted", interrupted, "contractLen", len(contract))
		}
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
	resumeUnattendedGates(ctrl, sessionPath)
	slog.Info("desktop: unattended session resumed after an interrupted host run", "session", sessionPath)
	return true
}

// resumeUnattendedGates takes down the gates a crash leaves behind. After a recovered
// queue the inbox is paused, and the resume turn would then sit as 待处理引导 until
// somebody clicks 继续执行 — and nobody is there in an unattended run. The master switch
// decides whether the session is unattended; a human being present changes nothing.
func resumeUnattendedGates(ctrl control.SessionAPI, sessionPath string) {
	guards, ok := ctrl.(heartbeatSessionGuards)
	if !ok {
		return
	}
	if guards.PlanMode() {
		guards.SetPlanMode(false)
	}
	if guards.InboxSnapshot().Paused {
		if err := guards.SetInboxPaused(false); err != nil {
			slog.Warn("desktop: resume the inbox of an unattended session", "err", err, "session", sessionPath)
			return
		}
		slog.Info("desktop: unattended session resumed its paused inbox", "session", sessionPath)
	}
}
