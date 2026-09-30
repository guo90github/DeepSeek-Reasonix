// Window handoff — when a long task's context window is spent, the driver moves
// the task to a fresh session, carrying the Goal contract so the new session
// anchors the same contract on its first tick.

package main

import (
	"log"
	"strings"
)

// unattendedHandoffPercent is the context usage at which the driver prefers a
// fresh session over one more turn against a full window.
const unattendedHandoffPercent = 96

// heartbeatContextUsage is the read-only usage half of the status port.
type heartbeatContextUsage interface {
	ContextSnapshot() (int, int)
}

// heartbeatWindowSpent decides the handoff from the context snapshot alone.
func heartbeatWindowSpent(used, window int) bool {
	if window <= 0 || used <= 0 {
		return false
	}
	return used*100 >= window*unattendedHandoffPercent
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

// handoffUnattendedTask moves a window-spent task to a fresh session and reports
// whether the caller should re-run this tick against the new topic.
func (e *HeartbeatEngine) handoffUnattendedTask(t *HeartbeatTask, ctrl heartbeatRuntimeStatus, scope, workspaceRoot, title string) bool {
	usage, ok := ctrl.(heartbeatContextUsage)
	if !ok {
		return false
	}
	used, window := usage.ContextSnapshot()
	if !heartbeatWindowSpent(used, window) {
		return false
	}
	oldPath := ""
	if sessions, ok := ctrl.(interface{ SessionPath() string }); ok {
		oldPath = sessions.SessionPath()
	}
	meta, err := e.app.CreateTopic(scope, workspaceRoot, title)
	if err != nil {
		log.Printf("[heartbeat] unattended %q could not open a handoff session: %v", t.Title, err)
		return false
	}
	e.mu.Lock()
	if e.handoffPreface == nil {
		e.handoffPreface = make(map[string]string)
	}
	e.handoffPreface[t.ID] = heartbeatHandoffPreface(*t, oldPath)
	e.mu.Unlock()
	t.TopicID = meta.ID
	log.Printf("[heartbeat] unattended %q moved to a fresh session (context %d/%d)", t.Title, used, window)
	return true
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
