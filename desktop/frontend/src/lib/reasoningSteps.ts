// Reasoning arrives as a free-text stream; providers never promise step
// structure, so step markers are heuristic. Splitting only on completed
// lines (terminated by \n) keeps the streaming view stable: a half-typed
// "Step 2:" never flickers into a step boundary, and the tail partial line
// always belongs to the active step.

export interface ReasoningStep {
  index: number; // 1-based display number
  title: string; // marker line minus its prefix, trimmed; falls back to "Step N"
  content: string; // text after the marker line, up to the next marker
  markerLine: string;
}

// Hard markers always split. "Step N" requires a number so prose like
// "Steps to take:" never matches.
const HARD_MARKER_RE = /^(?:Step\s*\d+|步骤\s*\d+|第\s*\d+\s*步|第[一二三四五六七八九十百]+步)\s*[:：—–.)]?\s*(.*)$/;
// Markdown numbered heading, e.g. "### 1. 分析" or "## Step 2: Verify".
const HEADING_MARKER_RE = /^#{1,6}\s+(?:Step\s*)?\d+\s*[.)、:：]\s*(.*)$/;
// Numbered list item — a soft marker, promoted only when the reasoning is
// genuinely list-shaped (several short items separated from prose).
const SOFT_MARKER_RE = /^\d+\s*[.)、]\s+(.+)$/;
const MAX_SOFT_MARKER_LEN = 40;

interface CompletedLine {
  text: string; // without trailing newline/CR
  start: number; // offset in text
  end: number; // offset just past the newline; next content starts here
  blank: boolean;
}

function completedLines(text: string): CompletedLine[] {
  const lines: CompletedLine[] = [];
  let start = 0;
  while (start < text.length) {
    const nl = text.indexOf("\n", start);
    if (nl === -1) break;
    let textEnd = nl;
    if (textEnd > start && text[textEnd - 1] === "\r") textEnd -= 1;
    const raw = text.slice(start, textEnd);
    lines.push({ text: raw, start, end: nl + 1, blank: raw.trim() === "" });
    start = nl + 1;
  }
  return lines;
}

interface PositionedMarker {
  line: CompletedLine;
}

// Soft markers split only when the list is real: at least two short items and
// each item starts on a blank line or right after another marker, so a
// numbered item buried mid-paragraph stays body text.
function positionedMarkers(text: string): PositionedMarker[] {
  const lines = completedLines(text);
  if (lines.length === 0) return [];
  const softCount = lines.reduce((n, line) => n + (SOFT_MARKER_RE.test(line.text) ? 1 : 0), 0);
  const markers: PositionedMarker[] = [];
  let prevBoundary = true;
  for (const line of lines) {
    if (HARD_MARKER_RE.test(line.text) || HEADING_MARKER_RE.test(line.text)) {
      markers.push({ line });
      prevBoundary = true;
    } else if (
      softCount >= 2 &&
      line.text.length <= MAX_SOFT_MARKER_LEN &&
      SOFT_MARKER_RE.test(line.text) &&
      prevBoundary
    ) {
      markers.push({ line });
      prevBoundary = true;
    } else {
      prevBoundary = line.blank;
    }
  }
  return markers;
}

function titleOf(marker: string, index: number): string {
  const hard = HARD_MARKER_RE.exec(marker);
  if (hard) return hard[1].trim() || `Step ${index}`;
  const heading = HEADING_MARKER_RE.exec(marker);
  if (heading) return heading[1].trim() || `Step ${index}`;
  const soft = SOFT_MARKER_RE.exec(marker);
  if (soft) return soft[1].trim() || `Step ${index}`;
  return `Step ${index}`;
}

/** All marker lines in order, regardless of count — drives the hook's timing. */
export function reasoningStepMarkers(text: string): string[] {
  return positionedMarkers(text).map((m) => m.line.text);
}

/**
 * Splits reasoning text into steps. Returns [] below two markers so callers
 * fall back to the flat view: one step is indistinguishable from prose.
 */
export function segmentReasoningSteps(text: string): ReasoningStep[] {
  const markers = positionedMarkers(text);
  if (markers.length < 2) return [];
  return markers.map((m, i) => ({
    index: i + 1,
    title: titleOf(m.line.text, i + 1),
    markerLine: m.line.text,
    content: text
      .slice(m.line.end, markers[i + 1] ? markers[i + 1].line.start : text.length)
      .trim(),
  }));
}
