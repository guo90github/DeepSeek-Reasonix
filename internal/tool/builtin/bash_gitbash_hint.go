package builtin

import (
	"runtime"
	"strings"

	"reasonix/internal/sandbox"
)

// posixShellTraps are the POSIX-shell traps that cost the model a round trip.
// They stay static text: the tool description is a cache-stable prefix, and the
// golden baseline in internal/boot requires it to be byte-identical per host, so
// no GOOS-conditional wording may reach it.
const posixShellTraps = "  - multi-line code: a bare 'python' can resolve through a Windows .bat shim that runs cmd.exe, which cuts the argument at the newline — a multi-line 'python -c \"...\"' then loses its code; use a heredoc (python - <<'PY'), a script file, or a one-line -c.\n" +
	"  - paths: a '/' path inside a script is not bash's /tmp on Windows (there bash's /tmp is the session temp dir, while a native .exe reads '/tmp' as C:\\tmp); prefer $TMP, a script-relative path, or cygpath -w on Git for Windows.\n"

// gitBashShimHint explains a multi-line -c payload lost inside the shim chain.
const gitBashShimHint = "hint: the interpreter's argument was cut at an embedded newline — in this git-bash a bare 'python' resolves through a .bat shim that runs under cmd.exe, which truncates its command line there, so the shim's own '||  goto :error' trailer became the -c code. Put multi-line code in a heredoc (python - <<'PY'), in a script file, or keep -c on one line."

// gitBashPathHint explains a '/' path a native Windows program could not read.
const gitBashPathHint = "hint: a '/' path inside a script or string is not this bash's /tmp — here bash's /tmp is the session temp directory, while a native Windows program reads '/tmp' as C:\\tmp. Use os.environ['TMP'], a path relative to the script file, or convert with cygpath -w."

// gitBashHint returns the recovery hint for a captured-output signature, or "".
// The shim signature demands both the batch trailer and a Python -c traceback,
// so cat-ing a .bat file is not misread as a failed command.
func gitBashHint(out string) string {
	switch {
	case strings.Contains(out, "goto :error") &&
		(strings.Contains(out, `File "<string>"`) || strings.Contains(out, "IndentationError")):
		return gitBashShimHint
	case strings.Contains(out, "FileNotFoundError") && hasQuotedRootPath(out):
		return gitBashPathHint
	}
	return ""
}

func hasQuotedRootPath(out string) bool {
	return strings.Contains(out, `'/`) || strings.Contains(out, `"/`)
}

// appendGitBashHints appends the git-bash recovery hint when captured output
// carries a shim-chain signature. A no-op for other hosts and shells.
func appendGitBashHints(out string, sh sandbox.Shell) string {
	return appendGitBashHintsOn(out, sh, runtime.GOOS)
}

func appendGitBashHintsOn(out string, sh sandbox.Shell, goos string) string {
	if goos != "windows" || !sh.Kind.IsPOSIX() {
		return out
	}
	if hint := gitBashHint(out); hint != "" {
		return appendSessionDataHint(out, hint)
	}
	return out
}
