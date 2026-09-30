// Key points of one answer, read back from the emphasis marks the ladder paints
// (md-p--claim / md-li--label). The marks are already in the rendered DOM and
// identical across the three markdown paths, so this reads one source of truth
// instead of re-parsing the text.

export type AnswerKeyPointKind = "claim" | "label";

// `ordinal` is the mark's address: its position among the answer body's marks in
// document order, written to the DOM as data-md-point. Dedupe and the display
// cap below never change it, so a jump target stays exact even when the same
// sentence is bolded twice.
export type AnswerKeyPoint = { ordinal: number; kind: AnswerKeyPointKind; text: string };

export type AnswerMarkSummary = { points: AnswerKeyPoint[]; total: number };

export const EMPTY_ANSWER_MARKS: AnswerMarkSummary = { points: [], total: 0 };

const CLAIM_CLASS = "md-p--claim";
const LABEL_CLASS = "md-li--label";
const SELECTOR = `.md p.${CLAIM_CLASS}, .md li.${LABEL_CLASS}`;
const MAX_POINTS = 6;
const MAX_CHARS = 120;

function collapse(text: string | null | undefined): string {
  return (text ?? "").replace(/\s+/g, " ").trim();
}

function truncate(text: string): string {
  return text.length <= MAX_CHARS ? text : `${text.slice(0, MAX_CHARS - 1).trimEnd()}…`;
}

// A labeled bullet's key information is its label, not the explanation that
// follows it, so the strip stays scannable.
function labelText(node: Element): string {
  const strong = node.querySelector(":scope > strong, :scope > p > strong");
  return collapse(strong?.textContent ?? node.textContent);
}

export function collectAnswerKeyPoints(root: ParentNode | null | undefined): AnswerMarkSummary {
  if (!root?.querySelectorAll) return EMPTY_ANSWER_MARKS;
  const points: AnswerKeyPoint[] = [];
  const seen = new Set<string>();
  let mark = 0;
  let total = 0;
  for (const node of root.querySelectorAll(SELECTOR)) {
    // Every mark gets its address before the strip decides what to list, so the
    // DOM and the strip agree on which mark "3" means. `total` counts reportable
    // points instead, so the header never overstates the answer.
    const ordinal = mark;
    mark += 1;
    node.setAttribute("data-md-point", String(ordinal));
    const kind: AnswerKeyPointKind = node.classList.contains(CLAIM_CLASS) ? "claim" : "label";
    const text = truncate(kind === "label" ? labelText(node) : collapse(node.textContent));
    if (!text || seen.has(text)) continue;
    seen.add(text);
    total += 1;
    if (points.length < MAX_POINTS) points.push({ ordinal, kind, text });
  }
  return { points, total };
}

export function sameAnswerKeyPoints(a: AnswerMarkSummary, b: AnswerMarkSummary): boolean {
  return a.total === b.total
    && a.points.length === b.points.length
    && a.points.every((point, index) => {
      const other = b.points[index];
      return point.ordinal === other.ordinal && point.kind === other.kind && point.text === other.text;
    });
}
