// Key points of one answer, read back from the emphasis marks the ladder paints
// (md-p--claim / md-li--label). The marks are already in the rendered DOM and
// identical across the three markdown paths, so this reads one source of truth
// instead of re-parsing the text.

export type AnswerKeyPointKind = "claim" | "label";

export type AnswerKeyPoint = { kind: AnswerKeyPointKind; text: string };

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

export function collectAnswerKeyPoints(root: ParentNode | null | undefined): AnswerKeyPoint[] {
  if (!root?.querySelectorAll) return [];
  const points: AnswerKeyPoint[] = [];
  const seen = new Set<string>();
  for (const node of root.querySelectorAll(SELECTOR)) {
    const kind: AnswerKeyPointKind = node.classList.contains(CLAIM_CLASS) ? "claim" : "label";
    const text = truncate(kind === "label" ? labelText(node) : collapse(node.textContent));
    if (!text || seen.has(text)) continue;
    seen.add(text);
    points.push({ kind, text });
    if (points.length >= MAX_POINTS) break;
  }
  return points;
}

export function sameAnswerKeyPoints(a: readonly AnswerKeyPoint[], b: readonly AnswerKeyPoint[]): boolean {
  return a.length === b.length && a.every((point, index) => point.kind === b[index].kind && point.text === b[index].text);
}
