// Presentation-only topic grouping for the session recap page.
//
// The merge happens in the view, never in the notes: every entry stays visible
// and every action applies to the entries the reader selected, so a wrong group
// costs a rearranged row rather than a note nobody ever sees. That is also why
// the rule below may be a plain keyword heuristic — the kernel dedupes exact
// repeats, and nothing here can hide content.
export type TopicTokens = Set<string>;

export type TopicGroup<T> = {
  key: string;
  entries: T[];
};

// Identifiers stay whole words; CJK becomes adjacent pairs, because a Chinese
// sentence restating a decision shares pairs rather than words.
export function topicTokens(body: string): TopicTokens {
  const tokens: TopicTokens = new Set();
  let ascii = "";
  let cjk: string[] = [];
  const flushASCII = () => {
    if (ascii.length >= 2) tokens.add(ascii);
    ascii = "";
  };
  const flushCJK = () => {
    for (let i = 0; i + 1 < cjk.length; i += 1) tokens.add(cjk[i] + cjk[i + 1]);
    cjk = [];
  };
  for (const char of body.toLowerCase()) {
    if (isCJK(char)) {
      flushASCII();
      cjk.push(char);
    } else if (/[\p{L}\p{N}_-]/u.test(char)) {
      flushCJK();
      ascii += char;
    } else {
      flushASCII();
      flushCJK();
    }
  }
  flushASCII();
  flushCJK();
  return tokens;
}

function isCJK(char: string): boolean {
  return /[\u3400-\u9fff\uf900-\ufaff\u3040-\u30ff]/u.test(char);
}

// A shared token only counts as a named subject when it is worth naming: a
// two-letter SQL alias (t1) or a bare id is exactly what two unrelated notes about
// one subsystem also share.
function isIdentifier(token: string): boolean {
  return token.length >= 4 && !isCJK(token[0]);
}

// sameTopic is deliberately strict, because its two mistakes do not cost the same:
// an over-eager group writes unrelated notes with one button, while a missed group
// only costs a second row. A shared subsystem word is therefore not enough — two
// real identifiers, or a shorter note nearly contained in the other (which is what
// a restatement looks like).
export function sameTopic(a: TopicTokens, b: TopicTokens): boolean {
  if (a.size === 0 || b.size === 0) return false;
  let shared = 0;
  let identifiers = 0;
  for (const token of a) {
    if (!b.has(token)) continue;
    shared += 1;
    if (isIdentifier(token)) identifiers += 1;
  }
  if (shared < 2) return false;
  if (identifiers >= 2) return true;
  if (shared >= 3 && identifiers >= 1) return true;
  return shared / Math.min(a.size, b.size) >= 0.8;
}

// groupByTopic keeps the incoming order: a group sits where its first entry sat,
// so the reader's list does not reshuffle when a note joins a topic. Kinds never
// share a group — a fact and a handoff about one topic land in different places,
// so one action row for both would be wrong.
export function groupByTopic<T extends { id: string; body: string; kind?: string }>(entries: T[]): TopicGroup<T>[] {
  const groups: TopicGroup<T>[] = [];
  const tokens: TopicTokens[] = [];
  const kinds: string[] = [];
  for (const entry of entries) {
    const entryTokens = topicTokens(entry.body);
    const kind = entry.kind ?? "";
    const index = tokens.findIndex((existing, at) => kinds[at] === kind && sameTopic(existing, entryTokens));
    if (index < 0) {
      groups.push({ key: entry.id, entries: [entry] });
      tokens.push(entryTokens);
      kinds.push(kind);
      continue;
    }
    groups[index].entries.push(entry);
    for (const token of entryTokens) tokens[index].add(token);
  }
  return groups;
}
