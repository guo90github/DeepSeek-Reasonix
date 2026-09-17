# Windows PowerShell shell mode

Reasonix runs the `bash` tool under Windows PowerShell only when no real Bash is
available, or when `[tools.shell] prefer` forces it. Auto-detection prefers Git
Bash (configured path, then `PATH`, then Git for Windows discovery), so a plain
`prefer = "auto"` with Git installed resolves to Bash. See `[tools.shell]` in
`SPEC.md`.

## What the host wraps around your command

Every command is launched as
`powershell -NoProfile -NonInteractive -Command "<prologue><command><trailer>"`:

- **UTF-8 prologue** — `$OutputEncoding` and `[Console]::OutputEncoding` become
  UTF-8 so command output survives a non-UTF-8 console code page (CP936 and
  friends).
- **Pinned text encodings** — `Get-Content` and `Out-File` default to `utf8` for
  the duration of the command: Windows PowerShell otherwise decodes
  UTF-8-without-BOM as ANSI (CJK mojibake on reads) and writes UTF-16LE from `>`
  / `Out-File`. `Set-Content` and `Add-Content` keep their ANSI default, because
  PowerShell's `utf8` mode prepends a BOM that breaks byte-exact consumers.
- **Exit-code trailer** — Windows PowerShell collapses every nonzero native exit
  code to `1`. The host appends a trailer that exits with the command's own
  status when the last statement was a native failure, so `rg` no-match (`1`),
  `rg` error (`2`) and `cmd /c exit 3` stay distinguishable.

Hook scripts do not get the trailer or the pinned encodings: their exit status
drives hook failure handling and must stay exactly what PowerShell reports.

## Traps and workarounds

| Trap | Symptom | Do this instead |
| --- | --- | --- |
| Wildcards reach native programs verbatim | `rg x *.go` → `os error 123` / "文件名、目录名或卷标语法不正确" | use that tool's own glob (`rg -g '*.go'`), the `glob`/`grep` tools, or pass a directory |
| `2>&1` | native stderr is rewritten into PowerShell error records (`所在位置 … CategoryInfo …`) | drop it — the host already merges stdout and stderr into one stream |
| `>` and `Out-File` | UTF-8 *with BOM* under the pinned encoding, or UTF-16LE without it | write source and JSON with `write_file` / `edit_file` when bytes matter |
| Native tools that print in the OEM code page | `net share`, `ipconfig`, `sc` come back garbled (`Ĭ�Ϲ���`) | prefix one command with `[Console]::OutputEncoding=[Text.Encoding]::GetEncoding(936);` |
| Piping text into a native program | the child receives a leading UTF-8 BOM | pass a file, or `cmd /c type file \| tool`; do not feed JSON through a pipe |
| A syntax error in a multi-line command | nothing runs and the parser error itself can be garbled (the prologue never executes) | keep commands small; re-run the pieces separately |
| `&&` / `\|\|` | Windows PowerShell 5.1 does not parse them | `;` (always runs both) or `if ($?) { … }`; PowerShell 7 accepts `&&` |
| `&>` | `AmpersandNotAllowed` parser error — PowerShell has no such operator | the host rewrites a null target (`&>nul`, `&>/dev/null`) to `*>`; for a real file write `*> file` yourself |
| `grep`, `sed`, `awk`, `head`, `tail` | absent, and `find` resolves to Windows' `find.exe` (not GNU find) | use `rg`, the `glob`/`grep` tools, `Select-String`, or the file tools |
| Several statements | a later success hides an earlier failure (`cmd /c exit 7; "x"` exits 0) | check the statement that matters; this matches Bash behavior |

## Related settings

- `[tools.shell] prefer` = `auto` | `bash` | `powershell` | `pwsh`, and
  `[tools.shell] path` for an explicit executable. Switching to Git Bash removes
  this whole class of traps.
- Execution policy: run `Get-ExecutionPolicy -List` before scripting with `.ps1`
  files — a `RemoteSigned` machine policy blocks downloaded scripts.
