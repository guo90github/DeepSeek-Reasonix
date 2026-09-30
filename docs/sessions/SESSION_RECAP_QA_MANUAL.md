# 会话回顾 · 桌面端真机验证说明书

适用：`v0.0.0-dev.77` 便携包（含本功能）已安装并在跑。全篇路径为这台机器的绝对路径。
读完只需按 §1 → §4 走一遍，§8 是要你打分的验收表。

---

## 0. 前提自证（先确认"这个包确实含本功能"）

| 证据 | 值 / 命令 | 结论 |
|---|---|---|
| 包内 build.json | `C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\app\resources\build.json` | version `v0.0.0-dev.77` / channel `stable` / commit `0a45842cad0b` / buildTime `2026-09-29T13:36:02Z` / electron `44.2.0` |
| 前端契约串在包内 | `rg -l "ListSessionRecaps" "C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\app\resources"` | 命中 `app\assets\bridge-*.js` → 页面与宿主命令都进了包 |
| **Go 侧宿主方法在桌面二进制里** | `rg -a -c "ListSessionRecaps" "…\versions\v0.0.0-dev.77\reasonix-desktop.exe"` | 命中 **7** 处 → 运行中的 App 真的带这个宿主命令 |
| 包内 CLI 含新命令 | `"C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\reasonix-cli.exe" history` | 打印 `usage: reasonix history <关键词> [--limit N] [--json]` |
| UI 自证（最快） | 打开 App → 侧栏应出现「会话回顾」入口（History 图标） | 出现 = 这份包就是新版 |

**这个包与 git 提交的关系**：build.json 的 `commit` 是构建时刻的 HEAD（`0a45842cad0b` = `fix(desktop): Git 未提交面板恒显示 0 个文件`）；
本功能是当时**工作树里未提交的改动**一起打进去的，随后落成三个提交：`9bcd33857`（内网地址脱敏）、`d670642d1`（会话回顾内核+接线+入口）、`7dd428f31`（设计文档）。
其中只有两处晚于这次构建：一处注释精简、一处把 boot 里的内联接线搬成同包函数（**行为等价**）。
因此**本次真机验证的结论对提交后的代码同样成立**；若要严格对齐，重打一次包再复跑 §1 即可。

当前激活版本记录在 `C:\Users\guosj\Reasonix-portable\current.json`（`activeVersion: v0.0.0-dev.77`）。

**本机运行期根目录（已核实，2026-09-29 21:40 仍在写）**

| 用途 | 路径 |
|---|---|
| 状态根（会话、config.toml、archive、sessions） | `C:\Users\guosj\AppData\Roaming\reasonix` |
| 缓存根（各投影 DB） | `C:\Users\guosj\AppData\Local\reasonix` |
| **回顾投影（本功能落盘处）** | `C:\Users\guosj\AppData\Local\reasonix\session-recap\v1.sqlite` |

> 注意：`C:\Users\guosj\Reasonix-portable\data`、`…\data2`、`C:\Users\guosj\AppData\Roaming\reasonix\cache`
> 都是 09-10 的**旧布局残留**，当前 App 不使用，别在那儿找投影。

**首次真机验证的结论（2026-09-30 09:02 那次，已定位并已修）**：投影文件存在，但当时 **0 条记录、0 条待补**。
原因是桌面关标签**先清空会话路径再关闭**（`desktop/tabs.go`），结束事件因此不带路径，回顾通道把空路径丢弃。
修法见 §10；**必须重打包才生效**（正在跑的 dev.77 早于这次修复）。

---

## 1. 最小闭环（先跑这条，约 5 分钟）

**目标**：一次关闭 → 页面出现一条能看懂的四要素回顾，且敏感串被遮蔽。

1. **造样本**：新建一个会话，聊 2–3 轮。内容里**故意**写进下面这些，用来验脱敏与要点提取：
   - 一个内网地址：`http://10.20.30.40:8080/admin`
   - 一个假 token：`sk-test-1234567890`
   - 一个明确待办：`明天给 X 模块补回归测试`
   - 一句结论：`原因是 A，不是 B`
2. **等回合跑完**（输入区不再有在跑的作业）。
   ⚠️ **有在飞作业时关闭标签 = 分离（作业进"后台作业"继续），不发会话结束事件 → 不生成回顾**。这是语义正确，不是 bug。
3. **关闭这个会话**（四个入口同源，都会触发）：
   - 快捷键 `Ctrl+W`（关闭标签页）
   - 标签总览菜单：**关闭标签页 / 关闭其他标签页 / 关闭右侧标签页**
4. **看回顾**：点侧栏「会话回顾」。列表为空就点右上角 **刷新**（回顾要真调一次模型，通常几秒）。
5. **预期**（逐项打勾）：
   - [ ] 出现一行，字段为 **生成时间 / 模型 / 目标 / 关键动作 / 结论 / 待办**（待办为空时不显示该行）
   - [ ] 四要素读得懂：说是哪个会话、干了什么、结论是什么、还剩什么
   - [ ] **`sk-test-1234567890` 与 `10.20.30.40` 都已被遮蔽**（应显示 `[redacted]`）
   - [ ] 「模型」= 该会话当时用的模型（不是别的）
   - [ ] 页面副标题为「只含会话关闭时生成的会话回顾；会话本身的删除、恢复在回收站。」

**开关语义对照（该生成 / 不该生成）**

| 动作 | 是否生成回顾 |
|---|---|
| 关闭（**无在飞作业**）本标签 / 关闭其他 / 关闭右侧 / 关闭标签页 | ✅ 生成 |
| 有在飞作业时关闭（分离，作业在"后台作业"里继续） | ❌ 不生成（语义正确） |
| `/clear` | ❌ 不生成（丢弃类动作） |
| 归档到回收站 / 彻底删除 | ❌ 不生成（丢弃类动作） |
| 重复关闭同一会话（内容未变） | ❌ 不重复（内容指纹幂等） |
| App 退出 | ❌ 不等（在飞回顾被放弃；下次打开不会补） |

---

## 2. 落盘佐证（只读，30 秒）

```bash
# 投影是否存在（生成成功后才出现）
dir "C:\Users\guosj\AppData\Local\reasonix\session-recap"

# 若上面不存在，两个候选一起找
cmd /c dir /s /b "%LOCALAPPDATA%\reasonix\session-recap" "%APPDATA%\reasonix\cache\session-recap"

# 记录条数与最近 5 条（path/model/生成时间/prompt 版本）
sqlite3 "C:\Users\guosj\AppData\Local\reasonix\session-recap\v1.sqlite" \
  "select count(*) from recap_records; \
   select path, model, datetime(generated_at,'unixepoch','localtime'), prompt_version from recap_records order by generated_at desc limit 5;"

# 待补队列（有行 = 没生成成功，见 §6）
sqlite3 "C:\Users\guosj\AppData\Local\reasonix\session-recap\v1.sqlite" "select * from recap_pending;"
```

判定：`recap_records` 有 1 行、`generated_at` 是你刚才关闭的时刻 → 落盘成功。
本机 `sqlite3` 为 3.50.6（已确认可用）。

---

## 3. 两条口径的实机核对

**3.1 归档到回收站 → 回顾页要"撤下"该行**（会话本身仍在回收站可恢复）

1. 挑一条**已生成回顾**的会话，把它移进回收站（会话列表右键 / 会话管理的删除）。
2. 回到「会话回顾」页 → 点刷新。
3. 预期：该行**从回顾页消失**；回收站里该会话仍在，可恢复。
4. 反向确认（记录仍在）：上一条 SQL 的 `count(*)` 不变（页是展示面，不是存储面）。

**3.2 回顾**不进**关键词检索（只作展示）**

```bash
# (a) 只在回顾里出现过、会话正文里没有的词 → 不应因回顾而命中
"C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\reasonix-cli.exe" history 回顾里的独有词

# (b) 会话正文里出现过的词 → 应命中，且命中行内带上该会话的回顾
"C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\reasonix-cli.exe" history 你写过的关键词
```

预期：(a) 报「无相关记录」；(b) 有命中，且每条命中下带 `会话回顾:（目标/关键动作/结论/待办）` 或「（尚无回顾）」。
机器可读：加 `--json`（`{query, hits[{sessionPath,title,time,score,snippet,recap{...}}]}`），条数可控 `--limit N`。

---

## 4. 通道隔离（本功能最硬的一条：不许打扰会话）

用户口径：**回顾的调用路径不得与会话抢占/共用，不得影响会话链路**。

1. **关闭要"瞬间"返回**：关闭一个已有若干轮历史的会话，动作应当立即完成（回顾在后台跑），加载圈不逗留。
2. **生成期间照常聊天**：关闭后马上在另一个会话发一轮消息并持续对话——消息时序、首字延迟、用量都应无变化（回顾用的是**独立 provider 实例**，不取会话租约、不进会话准入）。
3. **用量分账**：打开用量/费用面板，应能区分出**独立来源 `session-recap`**；它不并入会话用量、也不算标题生成。
4. **不进系统提示前缀**：会话系统提示与工具清单不因本功能改变（无新增 Agent 工具、无 tool schema 改动）。

---

## 5. 存量补录（可选；慢、真花钱、可中断可续跑）

给"以前就存在的会话"补生成回顾：

```bash
set CL="C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\reasonix-cli.exe"
%CL% catalogs reindex session-recap                 # 全部可回顾会话
%CL% catalogs reindex session-recap --dir <某目录>  # 只跑指定目录（可重复传）
%CL% catalogs reindex session-recap --json          # 机器可读统计
```

- 逐条**串行**真调模型 → 会话多时耗时按分钟计；内容指纹一致（已生成过）的会跳过，所以**可以随时中断、重跑续做**。
- 不可回顾的会话（不含系统恢复副本、不在可见集合、已移除）会被跳过并计入统计的三态。
- 建议在 App 不繁忙时跑，或干脆先退 App 再跑。

---

## 6. 症状 → 排查 → 处置

| 症状 | 先看哪里 | 处置 |
|---|---|---|
| 关闭后回顾页一直「还没有会话回顾」 | ①关的是不是**有在飞作业**的标签（= 分离）②`recap_pending` 里有没有该会话③App 是不是被你立刻退出了 | 让作业跑完再关；③ 让 App 多留几十秒 |
| `recap_pending` 有该会话行 | 模型没解析出来（既不认会话记录的模型，也没配回退） | 见下一行 |
| 需要配回退模型 | `C:\Users\guosj\AppData\Roaming\reasonix\config.toml` 的 `[agent]` 段 | 加一行 `session_recap_model = "deepseek-pro/deepseek-v4-pro"`（用你 config 里**真实存在**的 provider/model 名，格式 `provider/model`），存盘后**重启 App** |
| 想确认"到底生成过几条/哪几条" | §2 的 SQL | `recap_records` = 已生成；`recap_pending` = 待补 |
| 回顾正文里出现真实内网地址 / 密钥 | —— | **阻塞项**，立即记录原会话与截图报给本会话（脱敏是硬要求） |

⚠️ 改 `config.toml` 请用能保 UTF-8 的编辑器（VS Code / 记事本另存 UTF-8）。**不要用 PowerShell 的 `Get-Content`+`Set-Content` 回写**——那会按 GBK 解码/编码，把中文与符号写成乱码。

---

## 7. 本版不做（别当 bug 报）

- 回顾**不进**关键词检索索引（FTS）；桌面**没有**"按某个会话看它的回顾"的入口（只有整页列表 + CLI 检索）
- 回顾页**不做**删除/恢复（那是回收站的职责，副标题已写明）
- 服务端/手机端**不暴露**回顾检索
- 会话忙时的"让路判定"**未接线**：回顾不抢占会话，但也不会因会话忙而主动排队让路（闸门仍是串行）
- App 退出不等在飞回顾（投影可弃，下一轮会重新生成/补录）

---

## 8. 验收记录表（PRD §8 的 20 条抽查）

跑够样本后填这张表（可用 §5 的补录批量造样本）：

| # | 会话（时间 + 主题） | 生成时间 | 模型 | 四要素齐 | 凭回顾看懂(是/否) | 泄密/内网(有/无) | 备注 |
|---|---|---|---|---|---|---|---|
| 1 |  |  |  |  |  |  |  |
| 2 |  |  |  |  |  |  |  |
| … |  |  |  |  |  |  |  |
| 20 |  |  |  |  |  |  |  |

**判定**：`凭回顾看懂` ≥ 16/20（80%）即通过；「泄密/内网」列只要出现 1 个 `有` → **不通过**（阻塞项，先修脱敏）。

---

## 9. 一分钟速查

```bash
CL="C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.77\reasonix-cli.exe"
DB="C:\Users\guosj\AppData\Local\reasonix\session-recap\v1.sqlite"

$CL history 关键词                 # 关键词命中 + 行内回顾
$CL catalogs reindex session-recap # 存量补录（慢、花钱、可中断）
sqlite3 "$DB" "select count(*) from recap_records;"   # 生成了几条
sqlite3 "$DB" "select * from recap_pending;"          # 待补（没生成成功的）

# 配置回退模型（仅当会话记录的模型解析不出来时需要）：
#   C:\Users\guosj\AppData\Roaming\reasonix\config.toml → [agent] 段
#   session_recap_model = "deepseek-pro/deepseek-v4-pro"
```
