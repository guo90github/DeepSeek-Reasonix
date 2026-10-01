// Window handoff — when a long task's context window is spent, the driver moves
// the task to a fresh session, carrying the Goal contract so the new session
// anchors the same contract on its first tick.

package main

import (
	"fmt"
	"log"
	"strings"
)

// unattendedHandoffPercent is the default context usage at which the driver
// prefers a fresh session over one more turn against a full window.
const unattendedHandoffPercent = 90

// Handoff bounds: a hand-edited config must not be able to disable the handoff
// or fire it against a session that still has room.
const (
	minUnattendedHandoffPercent = 50
	maxUnattendedHandoffPercent = 99
)

// normalizeUnattendedHandoffPercent maps an unset value onto the default and
// clamps the rest into range.
func normalizeUnattendedHandoffPercent(percent int) int {
	if percent <= 0 {
		return unattendedHandoffPercent
	}
	return min(max(percent, minUnattendedHandoffPercent), maxUnattendedHandoffPercent)
}

// heartbeatContextUsage is the read-only usage half of the status port.
type heartbeatContextUsage interface {
	ContextSnapshot() (int, int)
}

// heartbeatContextExhausted reports a session whose last turn already died on
// the provider's window. It is the only signal left when the model never
// reported a window size, where the percentage check stays false forever.
type heartbeatContextExhausted interface {
	ContextExhausted() bool
}

// handoffPercentValue is the snapshotted threshold, normalized on read so a
// hand-edited config cannot disable or over-eager the handoff.
func (e *HeartbeatEngine) handoffPercentValue() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return normalizeUnattendedHandoffPercent(e.handoffPercent)
}

// heartbeatWindowSpent decides the handoff from the context snapshot and the
// configured threshold alone.
func heartbeatWindowSpent(used, window, percent int) bool {
	if window <= 0 || used <= 0 {
		return false
	}
	return used*100 >= window*normalizeUnattendedHandoffPercent(percent)
}

// heartbeatContextNumbers is the usage half of the status port, zero when the
// controller has no numbers to report.
func heartbeatContextNumbers(ctrl heartbeatRuntimeStatus) (int, int) {
	usage, ok := ctrl.(heartbeatContextUsage)
	if !ok {
		return 0, 0
	}
	return usage.ContextSnapshot()
}

// heartbeatSessionSpent reports whether this session must be left behind: spent
// by the numbers, or already failed on the window by the provider's own answer.
func heartbeatSessionSpent(ctrl heartbeatRuntimeStatus, percent int) bool {
	if exhausted, ok := ctrl.(heartbeatContextExhausted); ok && exhausted.ContextExhausted() {
		return true
	}
	used, window := heartbeatContextNumbers(ctrl)
	return heartbeatWindowSpent(used, window, percent)
}

// heartbeatSpentWindow is the switch-gated read of the same decision. It is
// consulted before the Goal hold: a running Goal drives its own turns, so a
// check placed behind that hold never fires for the long run it exists for.
func heartbeatSpentWindow(ctrl heartbeatRuntimeStatus, unattended bool, percent int) bool {
	if !unattended || ctrl == nil {
		return false
	}
	return heartbeatSessionSpent(ctrl, percent)
}

// heartbeatHandoffPreface tells the new session what it continues. The previous
// transcript stays on disk, so the model reads what it needs instead of the
// driver inventing a summary.
func heartbeatHandoffPreface(task HeartbeatTask, oldSessionPath string) string {
	var b strings.Builder
	b.WriteString("[接续会话] 上一会话的上下文窗口已用尽，这里接续同一目标继续推进。\n")
	if contract := strings.TrimSpace(task.Goal); contract != "" {
		b.WriteString("目标契约：" + contract + "\n")
	}
	if path := strings.TrimSpace(oldSessionPath); path != "" {
		b.WriteString("上一会话的完整记录在：" + path + "（需要时可读取）\n")
	}
	b.WriteString("请先确认目标与最近进展，再从下一步继续。")
	return b.String()
}

// heartbeatHandoffNotice is what the session left behind shows, so a person
// reopening it learns where the task went instead of finding silence.
func heartbeatHandoffNotice(newTitle string) string {
	return "上下文窗口已用尽：任务已交接给新会话「" + newTitle + "」，本会话不再推进。"
}

// handoffUnattendedTask moves a spent task to a fresh session. It reports the
// new session's title so the old one can say where the task went, and whether
// the caller should re-run this tick against the new topic.
func (e *HeartbeatEngine) handoffUnattendedTask(t *HeartbeatTask, ctrl heartbeatRuntimeStatus, scope, workspaceRoot, title string) (string, bool) {
	used, window := heartbeatContextNumbers(ctrl)
	if !heartbeatSessionSpent(ctrl, e.handoffPercentValue()) {
		return "", false
	}
	oldPath := ""
	if sessions, ok := ctrl.(interface{ SessionPath() string }); ok {
		oldPath = sessions.SessionPath()
	}
	meta, err := e.app.CreateTopic(scope, workspaceRoot, title)
	if err != nil {
		log.Printf("[heartbeat] unattended %q could not open a handoff session: %v", t.Title, err)
		return "", false
	}
	e.mu.Lock()
	if e.handoffPreface == nil {
		e.handoffPreface = make(map[string]string)
	}
	e.handoffPreface[t.ID] = heartbeatHandoffPreface(*t, oldPath)
	e.mu.Unlock()
	t.TopicID = meta.ID
	why := fmt.Sprintf("context %d/%d", used, window)
	if window <= 0 {
		why = "the last turn died on the context window"
	}
	log.Printf("[heartbeat] unattended %q moved to a fresh session (%s)", t.Title, why)
	return meta.Title, true
}

// takeHandoffPrompt consumes the one-shot preface a handoff left for this task.
func (e *HeartbeatEngine) takeHandoffPrompt(t HeartbeatTask) string {
	e.mu.Lock()
	prefix := e.handoffPreface[t.ID]
	delete(e.handoffPreface, t.ID)
	e.mu.Unlock()
	if strings.TrimSpace(prefix) == "" {
		return t.Prompt
	}
	return prefix + "\n\n" + t.Prompt
}
