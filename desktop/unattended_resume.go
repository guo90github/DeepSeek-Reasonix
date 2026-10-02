package main

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"reasonix/internal/control"
	"reasonix/internal/provider"
)

// unattendedResumeSource marks the host's own resume turn, so the inbox listing and the
// context builder can tell it apart from the work a person queued.
const unattendedResumeSource = "unattended-resume"

// An interrupted unattended session is asked to carry on, because nothing else does: the
// Goal, the board and the parked work all sit still until a turn arrives (2026-10-03). What
// that turn says matters — a bare 继续 tells the model nothing about what it was doing and
// invites it to improvise, so the host names the contract, the instruction it was on and the
// work the crash left queued instead.

// unattendedResumeContext is what the host knows about where a session stopped.
type unattendedResumeContext struct {
	Contract string
	LastUser string
	Pending  []string
}

func unattendedResumeText(ctx unattendedResumeContext) string {
	var b strings.Builder
	b.WriteString("【无人值守恢复】上一次宿主运行被中断（非正常结束）。这是接续那一轮，不是新指令。\n")
	if contract := strings.TrimSpace(ctx.Contract); contract != "" {
		fmt.Fprintf(&b, "- 任务契约（Goal）：%s\n", clipRunes(contract, 240))
	}
	if last := strings.TrimSpace(ctx.LastUser); last != "" {
		fmt.Fprintf(&b, "- 中断前在处理的指令：%s\n", clipRunes(last, 240))
	}
	if len(ctx.Pending) > 0 {
		fmt.Fprintf(&b, "- 队列里待处理的工作：%s\n", strings.Join(ctx.Pending, "；"))
	}
	b.WriteString("请先核对已完成的步骤（待办、看板、工作区现状），从中断点继续；" +
		"不要重做已完成的工作，也不要偏离上面的契约另起炉灶。契约已完成就直接给出结论。")
	return b.String()
}

// unattendedResumeContextFor gathers that context off the controller: the last real user
// instruction (host-generated user-role messages — a previous resume, a board wake — are not
// what the session was told to do) and the work the recovered queue still holds.
func unattendedResumeContextFor(ctrl control.SessionAPI, contract string) unattendedResumeContext {
	ctx := unattendedResumeContext{Contract: contract, LastUser: lastRealUserInstruction(ctrl)}
	guards, ok := ctrl.(heartbeatSessionGuards)
	if !ok {
		return ctx
	}
	for _, item := range guards.InboxSnapshot().Items {
		if item.Source == unattendedResumeSource {
			continue
		}
		if text := strings.TrimSpace(item.Preview); text != "" {
			ctx.Pending = append(ctx.Pending, clipRunes(text, 160))
		}
		if len(ctx.Pending) == 2 {
			break
		}
	}
	return ctx
}

func lastRealUserInstruction(ctrl control.SessionAPI) string {
	history := ctrl.History()
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if msg.Role != provider.RoleUser || msg.Origin == provider.MessageOriginHost {
			continue
		}
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			text = strings.TrimSpace(msg.RawContent)
		}
		if text != "" {
			return text
		}
	}
	return ""
}

func clipRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

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
	text := unattendedResumeText(unattendedResumeContextFor(ctrl, contract))
	if _, err := ctrl.TryEnqueueFollowup(control.InboxRequest{
		Submit:      text,
		Raw:         text,
		Source:      unattendedResumeSource,
		Idempotency: key,
	}); err != nil {
		slog.Warn("desktop: queue the unattended resume turn", "err", err, "session", sessionPath)
		return false
	}
	resumeUnattendedGates(ctrl, sessionPath)
	slog.Info("desktop: unattended session resumed after an interrupted host run",
		"session", sessionPath, "chars", len(text))
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
