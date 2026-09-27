// The panel's reading of one room line's answer. Whether a line ran is its own
// column now (the host records how it left the queue), so the panel reads that
// column instead of inferring an ending from a receipt lookup.

export type RoomLineRow = {
  state?: string;
  settled?: string;
  settledAt?: string;
};

export type RoomLineAnswer = {
  found?: boolean;
  line?: RoomLineRow | null;
} | null | undefined;

export type RoomLineExit =
  | { kind: "queued"; state: string }
  | { kind: "settled"; settled: string; settledAt?: string }
  | { kind: "never" };

// Only the three words the host actually writes get a gloss; anything else is
// passed through as its own word rather than explained into a claim nobody made.
const settledGloss: Record<string, string> = {
  acknowledged: "跑完并确认",
  discarded: "被取消",
  deleted: "被删掉",
};

export function roomLineExit(answer: RoomLineAnswer): RoomLineExit {
  const line = answer?.line ?? null;
  if (answer?.found) {
    return { kind: "queued", state: (line?.state || "").trim() };
  }
  const settled = (line?.settled || "").trim();
  if (settled) {
    const settledAt = (line?.settledAt || "").trim();
    return settledAt ? { kind: "settled", settled, settledAt } : { kind: "settled", settled };
  }
  return { kind: "never" };
}

export function settledLabel(settled: string): string {
  const word = (settled || "").trim();
  if (!word) return "";
  const gloss = settledGloss[word];
  return gloss ? `${word}（${gloss}）` : word;
}
