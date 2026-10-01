// Fixture for bench/recap-page-layout.mjs: the 会话回顾 page with a rail that is
// long enough to squeeze the card list if the height contract regresses (§2.4 of
// docs/60-第十四). Layout can only be judged in a real engine, so the page is
// mounted in Chromium through the Vite dev server and measured there.
import { createRoot } from "react-dom/client";
import "../src/styles.css";
import { LocaleProvider } from "../src/lib/i18n";
import { SessionRecapPage } from "../src/components/SessionRecapPage";
import type { RecapOpenItem, SessionMeta, SessionRecap, SessionRecapEntry, SessionRecapInsight } from "../src/lib/types";

const CARD_COUNT = 60;
const sessionPath = (index: number) => `C:\\sessions\\202609${String((index % 28) + 1).padStart(2, "0")}-0900${index % 10}00.000000000-deepseek-flash.jsonl`;

function entry(index: number, kind: SessionRecapEntry["kind"], body: string): SessionRecapEntry {
  return {
    id: `${kind}-${index}`,
    kind,
    body,
    target: kind === "handoff" ? "display" : "memory",
    evidence: `internal/recap/store.go:${100 + index}`,
    refs: [{ kind: "path", value: `internal/recap/file${index}.go`, detail: `L${20 + index}` }],
    scope: kind === "fact" ? "generic" : undefined,
    observedIn: kind === "fact" && index % 3 === 0 ? ["c--guosj-ai-chatting", "c--guosj-idea-workspace"] : undefined,
  } as SessionRecapEntry;
}

// The newest card carries many notes of one topic, which is the widest a single
// row can get: the group header, the checkboxes and the "N more" toggle.
const recaps: SessionRecap[] = Array.from({ length: CARD_COUNT }, (_, index) => {
  const notes: SessionRecapEntry[] = index === 0
    ? [
      ...Array.from({ length: 8 }, (_, note) => entry(note, "fact", `session_recap heatmapDay label regression ${note}`)),
      entry(90, "handoff", `把这段交接留给下一个会话（${index}）`),
    ]
    : [
      entry(index, "fact", `第 ${index} 次会话沉淀下来的结论，附上它的依据与出处。`),
      entry(index + 500, "handoff", `第 ${index} 次会话未完成的交接。`),
    ];
  return {
    path: sessionPath(index),
    model: "deepseek/deepseek-flash",
    generatedAt: new Date(Date.UTC(2026, 8, (index % 28) + 1, 9, 0, 0)).toISOString(),
    stale: index % 10 === 3,
    entries: notes,
  } as SessionRecap;
});

const sessions: SessionMeta[] = [
  ...recaps.slice(0, 12).map((recap, index) => ({
    path: recap.path, preview: "", title: `会话 ${index} 的标题`, turns: 4 + index,
    turnsState: "complete", createdAt: 0, lastActivityAt: 0, modTime: 0, current: false, open: false,
  } as SessionMeta)),
  ...Array.from({ length: 5 }, (_, index) => ({
    path: `C:\\sessions\\2026100${index + 1}-090000.000000000-deepseek-flash.jsonl`, preview: "",
    title: `还没回顾的会话 ${index}`, turns: 3 + index, turnsState: "complete",
    createdAt: 0, lastActivityAt: 0, modTime: 0, current: false, open: false,
  } as SessionMeta)),
];

const openItems: RecapOpenItem[] = Array.from({ length: 4 }, (_, index) => ({
  id: `open-${index}`,
  body: `留给下一个会话的未完成项 ${index}：把这条做完再收工。`,
  evidence: `internal/recap/store.go:${200 + index}`,
  from: sessionPath(index),
  openedAt: "2026-09-20T09:00:00Z",
  closed: index === 1,
  stale: index === 2,
  ageDays: index === 2 ? 60 : 3,
} as RecapOpenItem));

const insights: SessionRecapInsight[] = Array.from({ length: 3 }, (_, index) => ({
  kind: index % 2 === 0 ? "root-cause" : "refuted",
  body: `跨项目复现的结论 ${index}：只有卡片列表允许随条数变高。`,
  projects: ["c--guosj-ai-chatting", "c--guosj-idea-workspace"],
  occurrences: 2 + index,
  seenAt: "2026-09-30T09:00:00Z",
} as SessionRecapInsight));

const noop = async () => undefined;

createRoot(document.getElementById("root") as HTMLElement).render(
  <LocaleProvider>
    <SessionRecapPage
      active
      onBack={() => undefined}
      list={async () => recaps}
      listSessions={async () => sessions}
      resume={() => undefined}
      accept={async () => "recap-note.md"}
      reject={noop}
      undo={noop}
      listOpenItems={async () => openItems}
      keep={async () => "handoff-bench"}
      close={noop}
      reopen={noop}
      generate={async () => true}
      draftSkill={async () => ({ name: "bench", path: ".reasonix/skills/bench/SKILL.md" })}
      draftTopicSkill={async () => ({ name: "bench-topic", path: ".reasonix/skills/bench-topic/SKILL.md" })}
      previewMemory={async (source: { kind: string; body: string }) => ({ kind: "memory", text: source.body, fallback: source.body, promptTag: "memory-v1", model: "bench/model" })}
      previewSkill={async () => ({ kind: "skill", text: "# Bench", fallback: "1. step", promptTag: "skill-v1", model: "bench/model" })}
      listInsights={async () => insights}
    />
  </LocaleProvider>,
);
