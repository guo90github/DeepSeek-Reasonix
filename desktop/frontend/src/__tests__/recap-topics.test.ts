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

check("identifiers carry the topic", sameTopic(topicTokens("git commit message 一律用中文书写"), topicTokens("commit message 请用中文写")));
check("a lone shared word is not a topic",
  !sameTopic(topicTokens("打包脚本要用指定 bash"), topicTokens("打包产物放在 dist 目录")));

console.log(failed === 0 ? "OK: 0 failed" : `OK: ${failed} failed`);
if (failed > 0) process.exit(1);
