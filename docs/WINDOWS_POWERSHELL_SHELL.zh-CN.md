# Windows PowerShell 模式

只有当本机没有可用的真 Bash（或 `[tools.shell] prefer` 强制指定）时，Reasonix 才会把
`bash` 工具跑在 Windows PowerShell 下。自动探测优先 Git Bash（先看配置路径，再看
`PATH`，最后走 Git for Windows 的发现顺序），所以装了 Git 且用默认
`prefer = "auto"` 时会解析到 Bash。配置项见 `SPEC.md` 的 `[tools.shell]`。

## 宿主在命令外面包了什么

每条命令都以 `powershell -NoProfile -NonInteractive -Command "<prologue><command><trailer>"`
启动：

- **UTF-8 序言（prologue）**：把 `$OutputEncoding` 与 `[Console]::OutputEncoding` 设为
  UTF-8，让输出在非 UTF-8 控制台代码页（如 CP936）下不至于变成乱码。
- **固定文本编码**：命令执行期间 `Get-Content` 与 `Out-File` 的默认编码为 `utf8`。
  Windows PowerShell 本来会把**无 BOM 的 UTF-8** 当 ANSI 解码（中文读出来是乱码），
  并且 `>` / `Out-File` 默认写出 **UTF-16LE**（下游工具全废）。`Set-Content` 与
  `Add-Content` 保持 ANSI 默认：它们的 `utf8` 模式会加 BOM，破坏字节精确的消费者。
- **退出码尾处理（trailer）**：Windows PowerShell 会把所有非零的原生退出码压成 `1`。
  宿主在命令后追加一段尾处理：当最后一条语句是原生命令且失败时，用命令自己的退出码
  退出。于是 `rg` 无匹配（1）、`rg` 出错（2）、`cmd /c exit 3` 三者不再糊成一个 1。

Hook 脚本**不套**尾处理与固定编码：它们的退出码决定 hook 的失败判定，必须保持
PowerShell 原始上报的状态。

## 常见坑与写法

| 坑 | 现象 | 正确写法 |
| --- | --- | --- |
| 通配符被原样传给原生程序 | `rg x *.go` → `os error 123` /「文件名、目录名或卷标语法不正确」 | 用该工具自己的通配符（`rg -g '*.go'`）、`glob`/`grep` 工具，或直接给目录 |
| `2>&1` | 原生 stderr 被改写成 PowerShell 错误记录（`所在位置 … CategoryInfo …`） | 不要写：宿主已经把 stdout/stderr 合并成一条流 |
| `>` 与 `Out-File` | 固定编码下写成 UTF-8 **带 BOM**，不固定时是 UTF-16LE | 字节要精确时用 `write_file` / `edit_file` 写源码与 JSON |
| 按 OEM 代码页输出的原生工具 | `net share`、`ipconfig`、`sc` 的中文变乱码（`Ĭ�Ϲ���`） | 单条命令前加 `[Console]::OutputEncoding=[Text.Encoding]::GetEncoding(936);` |
| 往原生程序的 stdin 喂管道 | 子进程会收到一个 UTF-8 BOM 前缀 | 改用文件，或 `cmd /c type file \| tool`；JSON 不要走管道 |
| 多行命令里有语法错误 | 整条命令什么都不执行，而且解析错误本身也可能乱码（序言没机会执行） | 命令写小一点；分开重跑 |
| `&&` / `\|\|` | Windows PowerShell 5.1 不解析 | 用 `;`（两条都跑）或 `if ($?) { … }`；PowerShell 7 支持 `&&` |
| `&>` | 报 `AmpersandNotAllowed` 解析错误：PowerShell 没有这个运算符 | 目标是 null 时宿主会改写成 `*>`（`&>nul`、`&>/dev/null`）；写真实文件请自己写 `*> file` |
| `grep`、`sed`、`awk`、`head`、`tail` | 都不存在，且 `find` 解析到 Windows 的 `find.exe`（不是 GNU find） | 用 `rg`、`glob`/`grep` 工具、`Select-String` 或文件工具 |
| 多条语句串联 | 后面的成功会盖掉前面的失败（`cmd /c exit 7; "x"` 退出 0） | 单独检查关键那条；这与 Bash 行为一致 |

## 相关配置

- `[tools.shell] prefer` = `auto` | `bash` | `powershell` | `pwsh`，
  `[tools.shell] path` 指定具体可执行文件。切到 Git Bash 可以整类绕开上述坑。
- 执行策略：用 `.ps1` 脚本前先 `Get-ExecutionPolicy -List` —— 本机
  `LocalMachine` 为 `RemoteSigned`，下载来的脚本会被拦。
