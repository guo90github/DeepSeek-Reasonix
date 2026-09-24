package builtin

import (
	"fmt"
	"strings"

	"reasonix/internal/tool"
)

// Bounds for the current-text window a stale-anchor failure quotes back: the
// window spans the anchor's own lines only, so copying it back is the same edit
// against the current text rather than a replacement that drops context.
const (
	staleAnchorWindowLines = 8
	staleAnchorWindowBytes = 700
	staleAnchorLineBytes   = 200
	staleAnchorMinScore    = 4
	staleAnchorMinLine     = 3
)

// staleAnchorReport renders the current text the anchor most likely came from,
// so the rejected edit becomes one recoverable round. It returns "" when no
// window can be quoted faithfully: nothing in the file resembles the anchor, or
// the window would end at a final line without a newline.
func staleAnchorReport(oldString, content string) string {
	first, last, lines, ok := staleAnchorWindow(oldString, content)
	if !ok || !lineHasNewline(lines[len(lines)-1].raw) {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nnearest current text is lines %d-%d; action: %s; re-read that range (read_file offset=%d limit=%d, offset is 0-based), or copy the current text below and retry:\n",
		first+1, last+1, tool.RecoveryRereadTarget, first, last-first+1)
	for i, line := range lines {
		body := strings.TrimSuffix(strings.TrimSuffix(line.raw, "\n"), "\r")
		if len(body) > staleAnchorLineBytes {
			body = clipUTF8Prefix(body, staleAnchorLineBytes) + "\u2026[truncated]"
		}
		fmt.Fprintf(&b, "%4d\u2192%s\n", first+1+i, body)
	}
	return b.String()
}

// staleAnchorWindow aligns the anchor over the content and keeps the highest
// scoring window. Per-line common prefixes tolerate the drifts a stale anchor
// really shows, and the score only decides whether any window is worth quoting.
func staleAnchorWindow(oldString, content string) (first, last int, lines []lineSegment, ok bool) {
	contentLines := splitLineSegments(content)
	oldLines := splitLineSegments(oldString)
	if len(oldLines) == 0 || len(contentLines) == 0 {
		return 0, 0, nil, false
	}
	span := min(len(oldLines), len(contentLines))
	probes := make([]string, span)
	for i := range span {
		probes[i] = staleAnchorProbe(oldLines[i].raw)
	}
	bestStart, bestScore, bestLine, bestOffset := -1, 0, 0, 0
	for start := 0; start+span <= len(contentLines); start++ {
		score, line, offset := 0, 0, 0
		for k := range span {
			match := commonPrefixLen(staleAnchorProbe(contentLines[start+k].raw), probes[k])
			score += match
			if match > line {
				line, offset = match, k
			}
		}
		if score > bestScore {
			bestStart, bestScore, bestLine, bestOffset = start, score, line, offset
		}
	}
	if bestStart < 0 || bestScore < staleAnchorMinScore || bestLine < staleAnchorMinLine {
		return 0, 0, nil, false
	}
	// The best matching line bounds the shrink; a window that dropped it would
	// quote text the anchor never resembled.
	keep := min(bestStart+bestOffset, len(contentLines)-1)
	lo := bestStart
	hi := min(bestStart+len(oldLines), len(contentLines))
	if hi-lo > staleAnchorWindowLines {
		lo = max(lo, min(keep, hi-staleAnchorWindowLines))
		hi = min(lo+staleAnchorWindowLines, len(contentLines))
	}
	for hi-lo > 1 && staleAnchorLinesBytes(contentLines[lo:hi]) > staleAnchorWindowBytes {
		if hi-1 > keep {
			hi--
		} else if lo < keep {
			lo++
		} else {
			break
		}
	}
	return lo, hi - 1, contentLines[lo:hi], true
}

// staleAnchorProbe is the comparison form of one line: indentation, tabs and a
// copied read_file line prefix are normalization the match itself tolerates.
func staleAnchorProbe(raw string) string {
	body := strings.TrimSuffix(strings.TrimSuffix(raw, "\n"), "\r")
	body, _ = stripReadFileLinePrefix(body)
	return strings.TrimSpace(strings.ReplaceAll(body, "\t", "    "))
}

func staleAnchorLinesBytes(lines []lineSegment) int {
	total := 0
	for _, line := range lines {
		total += len(line.raw)
	}
	return total
}
