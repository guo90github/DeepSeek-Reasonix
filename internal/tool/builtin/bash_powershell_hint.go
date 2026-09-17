package builtin

import (
	"strings"

	"reasonix/internal/sandbox"
)

// psShellTraps are the PowerShell traps that cost the model a round trip: an
// unexpanded wildcard (Windows error 123) and '2>&1', which rewrites native
// stderr into PowerShell error records the host has already merged.
const psShellTraps = "  - redirect/vars: $null not /dev/null; $env:VAR not $VAR; '2>$null' drops stderr; no '2>&1' (stderr is already merged; it only adds error-record noise).\n" +
	"  - wildcards: PowerShell passes '*.go' to native programs verbatim (Windows error 123); use 'rg -g \"*.go\"', the glob/grep tools, or a directory.\n"

// psGlobHint explains a wildcard that reached a native program unexpanded:
// Windows reports ERROR_INVALID_NAME (os error 123) and the model needs the
// recovery, which the raw error text does not state.
const psGlobHint = "hint: Windows PowerShell does not expand wildcards for native commands, so the pattern was passed literally (Windows error 123). Put the glob in that tool's own flag (rg -g '*.go'), use the glob/grep tools, or pass the directory."

// appendPowerShellGlobHint appends psGlobHint when captured output carries the
// ERROR_INVALID_NAME signature. A no-op for other shells and clean output.
func appendPowerShellGlobHint(out string, sh sandbox.Shell) string {
	if sh.Kind != sandbox.ShellPowerShell {
		return out
	}
	for _, key := range []string{
		"os error 123",
		"文件名、目录名或卷标语法不正确",
		"The filename, directory name, or volume label syntax is incorrect",
	} {
		if strings.Contains(out, key) {
			return appendSessionDataHint(out, psGlobHint)
		}
	}
	return out
}
