# 回顾产出的两份提示词：记忆 与 技能

**一句话**：模型只被"回顾"提示词驱动一次，产出**条目（notes）**；**记忆**=条目本身，**技能**=把条目
逐字合成一份 playbook（**无模型、无提示词**，是代码模板）。所以"记忆提示词"与"技能提示词"不是两套
独立配置，而是**一份提示词 + 一个合成模板**。

## 位置总表

| 东西 | 权威位置 | 可否覆盖 | 版本标记 |
|---|---|---|---|
| 记忆（条目）走的**模型提示词** | `internal/recap/generator.go` 的 `recapSystemPrompt`（内置） | 可：把 `recap-prompt.md` 放进**本地状态目录**（与 `session-recap/v1.sqlite` 同目录） | 记录里写 `recap-v9`（覆盖时 `recap-v9+<8 位哈希>`） |
| 记忆落点（项目 / 全局） | 条目自身的 `scope` 提议 + 页面采纳时的分档开关 | 采纳时可选 | — |
| 技能（playbook）**合成模板** | `desktop/recap_skill.go` 的 `recapSkillMarkdown` / `recapTopicSkillMarkdown` | 不可（代码） | 产物写 `invocation: manual` |
| 回顾用的**模型** | 该会话记录的模型 → 配置项 `agent.session_recap_model` | 可（配置） | — |
| 页脚"记忆 / 技能建议" | `desktop/memory_suggestions.go`（**规则扫描，无模型**） | 不可 | — |

## 一、记忆那份：内置提示词全文（`recap-v9`）

> 权威来源是上面的代码常量。下面是**同一份文本的可读副本**；若要改动，请用覆盖文件而不是改副本。

```text
You distill one finished coding session into reusable notes for the next session.
Answer with a JSON array and nothing else, like this:
[{"kind":"fact","body":"one or two sentences","evidence":"where it came from","refs":[{"kind":"path","value":"desktop/x.go","detail":"L40"},{"kind":"command","value":"go test ./internal/recap/"}],"scope":{"level":"project","reason":"only true in this repository"}}]

Kinds, and what earns a note:
- fact: something durable about the project as it now stands — where a thing lives, a path that matters later, a protocol, a schema, a field's meaning.
- root-cause: a defect that was actually diagnosed. Write it as a procedure: what triggers it, what to check to confirm it, and the fix that removes the cause rather than the symptom.
- refuted: something the session tried, assumed or reached for and then dropped — an approach, a tool, a version, a hypothesis — with what ruled it out. Write it as a procedure too: what someone would try, and the check that shows why it fails. An option still under discussion is not refuted; an approach the session simply used is not refuted either.
- handoff: work left unfinished, and the next concrete step — a command, a file, a decision. "Continue the work" is not a handoff.

One note per decision, not per turn:
- A session states the same decision, rule or conclusion more than once as it goes. That is one note. Fold the later wording into the same body instead of emitting a second note, and put the strongest pointer in refs. Two notes that a reader would group as "the same thing" are one note.
- A session that revisits a decision and changes it ends with the changed one; the earlier version is not a note. Only a reversal that someone could re-propose earns a refuted note.

What earns a note at all:
- Prefer what only this session knows. The repo, its docs and its history answer their own questions on demand, so a note that restates a file, a commit or a document spends the next session's attention and returns nothing. What earns a note: what the person decided or ruled out, a dead end already walked, the cause of something that went wrong, a constraint nobody wrote down.
- Imagine the reader a month from now: if the note would not change what they do next, it is not a note.
- Report what the session settled on, never what it merely discussed, praised, planned or ran for the first time without a conclusion.

How to write one body:
- At most two sentences and at most 120 characters. The next session reads this as a list, not as a report: a note that needs more room is two notes.
- At most two identifiers in the body (a path, a command, an id). Everything else belongs in "evidence" or "refs".
- Never name a thing you cannot point at: every path, symbol, flag, version or decision number in a body must also appear in refs or evidence. If the session does not contain the wording, write the uncertainty instead of asserting it — a note that is confidently a little wrong costs more than no note, because the next session quotes it as fact.
- Copy names, identifiers, file paths, flags, version strings, numbers and field values verbatim. Never paraphrase, translate, shorten or round them (a range stays a range: "O1-O9" is not "O1-O6").
- Machine state (a timestamp, a memory address, a process count, "N tests pass right now") is worth a note only when the next session would otherwise re-derive it wrongly. Never dress such a thing as a root-cause.
- A note must stand without this session: no "as decided above", no "the earlier fix", no recounting of what was run, committed, checked or verified. No greetings, no restating the request, no narrating the conversation.
- Never include secrets, credentials, hostnames or internal addresses.
- Write each body in the session's own language; keep identifiers as they are.
- The transcript may be trimmed: never invent detail to fill a gap.

Before answering, scan the session for four things: a defect that was diagnosed, anything it dropped along the way, anything it left unfinished, and any decision it stated more than once (those merge). Each hit is one note; never drop a hit because the list is getting long, and never split one hit into several.
- Aim for three to six notes, never more than 8. A session that produced nothing reusable answers [].

For each note set "scope": project when it is true only here, base when it is true of the person's setup everywhere, generic when it is true of the work itself. Put the reason in "scope.reason" in one clause.
```

**它决定了什么**：条目写不写得准、能不能被追溯、值不值得记。技能草稿是从条目**逐字**长出来的，
所以**技能的质量上限=这份提示词的产出质量**（尤其"根因 / 否证按过程写"那一条）。

**怎么覆盖**：把 `recap-prompt.md` 放进本地状态目录（Windows 为 `%LOCALAPPDATA%easonix\`），
内容原样生效、记录版本变为 `recap-v9+<8 位哈希>`；文件为空、超 32 KiB 或读不出来时**回退内置并说明原因**。

## 二、技能那份：合成模板（确定性的，无提示词）

一份技能 = **该话题的全部条目逐字并置**：开头注明"来自 N 条同话题条目、未做改写或推断"，然后

- `## When it applies`：逐条列出条目正文（原文）
- `## What to do`：按会话产生顺序编号，每条后缀它的 kind、`Where to check:`（该条目自己的引用）与 `Evidence:`
- frontmatter：`name`（由首条正文生成）、`description`、**`invocation: manual`**

产物落在本项目 `.reasonix/skills/recap-*/SKILL.md`，**不覆盖已存在文件**，只能由人触发；
非过程类条目（事实、交接）被宿主拒绝。**不同话题各自一份**⇒ 批量天然可能是"一份"也可能是"多份"。

## 三、若你要"模型撰写的技能"（当前**没有**，需要新增）

现在的技能是**可追溯但不成文**（像步骤清单，不像人写的流程）。要让它"成文"，必须新增一次模型调用，
其提示词草案如下——**代价**是：可靠性从"逐字可追溯"变成"模型的理解"，且每条技能都要多一次调用与费用；
建议只在"你已确认该话题的条目正确"之后再点。若要做，我再落地（含宿主编排与页面入口）。

```text
You write one playbook from a set of verified notes about a single topic. The notes are
already accepted facts, diagnoses and ruled-out approaches; do not invent steps, files,
commands or numbers that are not in them, and do not re-explain why the notes are true.
Merge the notes into one procedure a person can follow without reading the session:
- Order the steps the way the work is actually done, not the way the notes were written.
- Each step says what to do and how to tell it worked; keep every path, command, flag and
  identifier exactly as the notes write them, and keep each note's pointers with the step
  they belong to.
- If two notes describe the same step, keep one step with both pointers.
- If a note is a rule rather than a step ("never X"), put it in a "Never" list at the end.
- If the notes disagree, keep the newer one and say in one line which older claim it replaces.
Answer as markdown only: a title, then "## Steps" (numbered), then "## Never" when you have
any, then "## Where to check" listing the pointers you actually used. No preamble, no summary
of the notes, no advice that is not in them. Write in the notes' own language.
```
