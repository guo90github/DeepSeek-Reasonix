import { groupByTopic, sameTopic, topicTokens } from "../lib/recapTopics";

let failed = 0;
function check(label: string, condition: boolean) {
  if (condition) return;
  failed += 1;
  console.error(`FAIL ${label}`);
}

const entries = [
  { id: "a", kind: "fact", body: "提交信息一律用中文书写，commit message 不要写英文" },
  { id: "b", kind: "fact", body: "packaging 脚本要用指定的 bash，别用 PowerShell 回写文件" },
  { id: "c", kind: "fact", body: "commit message 必须用中文写，别写英文" },
  { id: "d", kind: "handoff", body: "commit message 的中文要求还没落到文档里" },
];

const groups = groupByTopic(entries);
check("one topic keeps one group", groups.length === 3);
check("the group sits where its first entry sat", groups[0].key === "a");
check("the restatement joins the first group", groups[0].entries.map((entry) => entry.id).join(",") === "a,c");
check("a different topic is its own group", groups[1].entries.map((entry) => entry.id).join(",") === "b");
check("a handoff never joins a fact's group", groups[2].entries.map((entry) => entry.id).join(",") === "d");
check("nothing is hidden", groups.reduce((total, group) => total + group.entries.length, 0) === entries.length);

const restatements: [string, string][] = [
  ["提交信息一律用中文书写，commit message 不要写英文", "commit message 必须用中文写，别写英文"],
  ["打包时不要用 PowerShell 回写仓库文件", "别用 PowerShell 覆盖仓库里的文件"],
];
for (const [a, b] of restatements) {
  check(`one topic: ${a.slice(0, 12)}…`, sameTopic(topicTokens(a), topicTokens(b)));
}

// Two notes about one subsystem share an alias (t1) and a generic word or two, not
// a subject: grouping them would write unrelated notes with one button.
const apart: [string, string][] = [
  [
    "外层 SELECT * 拿不到 ID，是因为内层投影只把主表 ID 暴露成 COSTS_ID, FundingCenterVo.id 遂为 null；已在两处 select 各加 t1.ID",
    "queryListRealTime 构造 AccountQuery 时第 4 参传 null（即 broker），入参 simpleWhereCdt 根本没进 SQL，TRADE_FEE 分支只按 t1.STATUS=0 过滤",
  ],
  ["打包脚本要用指定的 bash", "打包产物要放在 dist 目录"],
];
for (const [a, b] of apart) {
  check(`apart: ${a.slice(0, 12)}…`, !sameTopic(topicTokens(a), topicTokens(b)));
}

console.log(failed === 0 ? "OK: 0 failed" : `OK: ${failed} failed`);
if (failed > 0) process.exit(1);
