package agent

import (
	"fmt"
	"strings"

	"reasonix/internal/i18n"
)

// truncateToolOutputFor is the tool-aware provider-input limiter. toolName and
// toolCallID populate the recovery marker.
func truncateToolOutputFor(s, toolName, toolCallID string) (string, string) {
	if len(s) <= maxToolOutputBytes {
		return s, ""
	}
	if toolName == "read_file" {
		return truncateReadFileOutput(s, toolName, toolCallID)
	}
	strategy := snipStrategy{head: 40, tail: 40, headChars: 8000, tailChars: 8000}
	switch {
	case toolName == "bash" || toolName == "shell" || strings.Contains(toolName, "bash"):
		strategy = snipStrategy{head: 40, tail: 40, headChars: 8000, tailChars: 8000}
	case toolName == "read_file" || toolName == "web_fetch" || strings.Contains(toolName, "read"):
		strategy = snipStrategy{head: 120, tail: 12, headChars: 12000, tailChars: 2000}
	case toolName == "grep" || toolName == "glob" || toolName == "ls" || toolName == "list_dir":
		strategy = snipStrategy{head: 80, tail: 8, headChars: 10000, tailChars: 1000}
	}
	headKeep := strategy.headChars
	tailKeep := strategy.tailChars
	if headKeep+tailKeep > maxToolOutputBytes-512 {
		headKeep = maxToolOutputBytes * 2 / 3
		tailKeep = maxToolOutputBytes - headKeep - 512
	}
	if headKeep < 1024 {
		headKeep = maxToolOutputBytes / 2
		tailKeep = maxToolOutputBytes / 2
	}
	// Prefer more tail when the body looks like a failure.
	lower := strings.ToLower(s)
	if strings.Contains(lower, "error:") || strings.Contains(lower, "panic:") || strings.Contains(lower, "fatal:") {
		tailKeep = max(tailKeep, maxToolOutputBytes/3)
		if headKeep+tailKeep > maxToolOutputBytes-512 {
			headKeep = maxToolOutputBytes - 512 - tailKeep
		}
	}
	head := snapToRuneBoundary(s, 0, headKeep)
	tail := snapToRuneBoundary(s, len(s)-tailKeep, len(s))
	resultRef := toolResultRef(toolCallID, s)
	marker := toolOutputRecoveryMarker(toolName, toolCallID, resultRef, len(s), len(head)+len(tail))
	for range 3 {
		bodyLen := len(head) + len(marker) + len(tail)
		if bodyLen <= maxToolOutputBytes {
			break
		}
		overflow := bodyLen - maxToolOutputBytes
		trimHead := overflow / 2
		trimTail := overflow - trimHead
		if trimHead < len(head) {
			head = snapToRuneBoundary(head, 0, len(head)-trimHead)
		}
		if trimTail < len(tail) {
			tail = snapToRuneBoundary(tail, trimTail, len(tail))
		}
		marker = toolOutputRecoveryMarker(toolName, toolCallID, resultRef, len(s), len(head)+len(tail))
	}
	notice := fmt.Sprintf(i18n.M.ToolOutputTruncatedFmt, len(s)-len(head)-len(tail), len(s))
	return head + marker + tail, notice
}
