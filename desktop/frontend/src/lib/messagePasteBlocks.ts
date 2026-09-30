import { formatSelectionLabels, parseSelectedTextContext } from "./selectedTextContext";

// Pasted and selected-text blocks are rebuilt from the markers the composer writes
// around them, so the transcript renders them as first-class cards instead of
// cutting them out of the message body.
export type PastedBlockInfo = {
  label: string;
  content: string;
};

const PASTE_LABEL_RE = /\[(?:已粘贴文本|已貼上文字|Pasted text) #\d+ · \d+ (?:行|lines)\]/g;

export function parsePastedBlocks(text: string, submitText?: string): PastedBlockInfo[] {
  const labels = text.match(PASTE_LABEL_RE);
  if (!labels || labels.length === 0 || !submitText) return [];
  const unique = [...new Set(labels)];
  const blocks: PastedBlockInfo[] = [];
  for (const label of unique) {
    const beginMarker = `--- Begin ${label} ---`;
    const endMarker = `--- End ${label} ---`;
    const beginIdx = submitText.indexOf(beginMarker);
    const endIdx = submitText.indexOf(endMarker);
    if (beginIdx < 0 || endIdx <= beginIdx) continue;
    const contentStart = beginIdx + beginMarker.length;
    const content = submitText.slice(contentStart, endIdx).replace(/^\r?\n/, "");
    blocks.push({ label, content });
  }
  return blocks;
}

export type SelectedTextBlockInfo = {
  label: string;
  content: string;
  path?: string;
  start: number;
  end: number;
  kind: "chat" | "code" | "terminal";
};

export function parseSelectedTextBlocks(text: string, submitText?: string): SelectedTextBlockInfo[] {
  const entries = parseSelectedTextContext(submitText);
  if (entries.length === 0) return [];
  const suffix = formatSelectionLabels(entries);
  if (!suffix || !text.endsWith(suffix)) return [];

  // Composer owns the exact trailing label suffix. Deriving it from the JSON
  // entries avoids consuming label-shaped or unterminated authored prose.
  let start = text.length - suffix.length;
  return entries.map((entry) => {
    const label = formatSelectionLabels([entry]);
    const kind = entry.path ? "code" : entry.source === "terminal" ? "terminal" : "chat";
    const block = {
      label,
      content: entry.text,
      path: entry.path,
      start,
      end: start + label.length,
      kind,
    } satisfies SelectedTextBlockInfo;
    start = block.end + 1;
    return block;
  });
}
