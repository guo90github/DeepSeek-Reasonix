# 本机桌面版打包 SOP（自用 · 常驻规则 · 机器本地文件，请勿提交）

适用触发：「打包」「打包新的桌面版程序」「打包新的 vX」——不论哪个会话，一律走下面这一条流程。
不要临场发明路径、tag 或报告格式；不要询问 tag、也不要问要不要装（规则已定）。

## 唯一流程

1. **定 tag**：读 `C:\Users\guosj\Reasonix-portable\versions` 下已有的 `v0.0.0-dev.N`，取最大 N **自动 +1**
   （已有 `.4` → `v0.0.0-dev.5`）。同一 tag 绝不重打，不加日期或后缀。
   若 `dist\` 里已有比 portable 更晚的产物（可能另一个会话刚打过），先停下报告，不要盲目重打。
2. **构建**（脚本是 bash，必须用这个 bash；NSIS 不在 PATH）：
   ```powershell
   cd C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix
   $env:PATH = "C:\Program Files (x86)\NSIS;" + $env:PATH
   $env:REASONIX_SKIP_BUDGET = "1"
   & "C:\soft\git\Git\bin\bash.exe" scripts/desktop-build.sh windows/amd64 <tag>
   ```
3. **装免安装**（并列新增，绝不删旧版）：
   `Expand-Archive dist\Reasonix-windows-amd64.zip -DestinationPath C:\Users\guosj\Reasonix-portable -Force`
   （刷新根 launcher，并把 `current.json` 指向新 tag）
4. **校验**：`& "C:\soft\git\Git\bin\bash.exe" scripts/verify-windows-portable.sh "C:\Users\guosj\Reasonix-portable"`
   —— **exit 0 才算成功**，非 0 必须如实报告。
5. **快照完整性**：构建前后对 `git status --porcelain` 列出的文件各算一次 SHA256 并比对；有变动就说明
   「本次包=中途快照，可能不含最新改动」，不得声称包含。

## 报告格式（固定，只写绝对路径）

- tag；已装版本目录绝对路径；日常入口 `C:\Users\guosj\Reasonix-portable\Reasonix.exe`
- `dist\Reasonix-windows-amd64-installer.exe` 与 `dist\Reasonix-windows-amd64.zip`：绝对路径 + 字节数 + 时间
- 包内 `versions\<tag>\app\resources\build.json` 的 version/channel/commit（证明打进去的是哪棵树）
- `verify-windows-portable.sh` 的 exit code
- 快照完整性结论；以及本次是否覆盖了别人的产物（不确定就说不确定）

不写未验证的路径，不用相对路径，不凭印象罗列"可能的中间产物"。

## 已知噪声（不要当失败）

- 构建任务末尾报 `exit status 1`：PowerShell 5.1 把 vite stderr 包成 `NativeCommandError`；只要脚本末条
  `ls dist` 正常执行且 verifier exit 0，就是成功。
- `desktop/electron` 单测里既有 5 条 Windows 路径分隔符失败，与改动无关。
- 本地尺寸闸门本就超出（如 app-shell CSS gzip ~122.5 KiB），本地一律配 `REASONIX_SKIP_BUDGET=1`；
  不要为了过闸门去改大 `desktop/frontend/scripts/check-bundle-budget.mjs`。

## 禁忌

- 不按进程名通配 `Stop-Process`（会杀掉你正在用的 App 与托管当前对话的进程）。
- 不覆盖、不删除已装的 `versions\*`；不触碰 `%APPDATA%\reasonix` 下的会话文件。
- 不把 `desktop\build\...` 下的中间产物当可点入口。
