package builtin

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

// gitBashShimMangledOutput is the field signature of 2026-09-25: git-bash's
// `python` resolved through pyenv-win's shim into cmd.exe, the multi-line -c
// payload was cut at its first newline, and the batch trailer became the code.
const gitBashShimMangledOutput = `error: command exited: exit status 1
which: no jq in (/mingw64/bin:/usr/bin)
/c/Users/guosj/.pyenv/pyenv-win/shims/python
  File "<string>", line 1
    ||  goto :error
IndentationError: unexpected indent
`

func gitBashTestShell() sandbox.Shell {
	return sandbox.Shell{Kind: sandbox.ShellBash, Path: `C:\Program Files\Git\bin\bash.exe`}
}

func TestGitBashHintMatchesShimMangle(t *testing.T) {
	if got := gitBashHint(gitBashShimMangledOutput); got != gitBashShimHint {
		t.Fatalf("hint = %q, want the shim hint", got)
	}
}

func TestGitBashHintMatchesNativePathMismatch(t *testing.T) {
	out := "Traceback (most recent call last):\n" +
		"  File \"trans.py\", line 5, in <module>\n" +
		"FileNotFoundError: [Errno 2] No such file or directory: '/tmp/chatroom/transcript.txt'\n"
	if got := gitBashHint(out); got != gitBashPathHint {
		t.Fatalf("hint = %q, want the path hint", got)
	}
}

func TestGitBashHintIgnoresUnrelatedOutput(t *testing.T) {
	for _, out := range []string{
		// Reading a .bat shows the trailer without any Python -c traceback.
		"set \"cmdline=%*\"\n%cmdline% ||  goto :error\n",
		"ok\n",
		"",
	} {
		if got := gitBashHint(out); got != "" {
			t.Fatalf("gitBashHint(%q) = %q, want none", out, got)
		}
	}
}

func TestAppendGitBashHintsGatesHostAndShell(t *testing.T) {
	cases := []struct {
		name string
		sh   sandbox.Shell
		goos string
		want bool
	}{
		{name: "git-bash on windows", sh: gitBashTestShell(), goos: "windows", want: true},
		{name: "bash on linux", sh: sandbox.Shell{Kind: sandbox.ShellBash, Path: "/bin/bash"}, goos: "linux", want: false},
		{name: "zsh on darwin", sh: sandbox.Shell{Kind: sandbox.ShellZsh, Path: "/bin/zsh"}, goos: "darwin", want: false},
		{name: "powershell on windows", sh: sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: "powershell"}, goos: "windows", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appendGitBashHintsOn(gitBashShimMangledOutput, tc.sh, tc.goos)
			if has := strings.Contains(got, gitBashShimHint); has != tc.want {
				t.Fatalf("hint present = %v, want %v", has, tc.want)
			}
			// The captured output itself must survive untouched.
			if !strings.Contains(got, "||  goto :error") {
				t.Fatalf("original output lost: %q", got)
			}
		})
	}
}

func TestAppendGitBashHintsKeepsCleanOutput(t *testing.T) {
	if got := appendGitBashHintsOn("build ok\n", gitBashTestShell(), "windows"); got != "build ok\n" {
		t.Fatalf("clean output changed: %q", got)
	}
}

func TestPosixShellTrapsAreStatic(t *testing.T) {
	for _, want := range []string{"heredoc", "python - <<'PY'", "cmd.exe", "cygpath -w"} {
		if !strings.Contains(posixShellTraps, want) {
			t.Fatalf("posix shell traps missing %q: %q", want, posixShellTraps)
		}
	}
}

// TestBashDescriptionCarriesTrapsForPosixShellsOnly pins the cache contract:
// the traps text is static (no GOOS input), so the prefix stays byte-identical
// per host, and the PowerShell branch never borrows POSIX advice.
func TestBashDescriptionCarriesTrapsForPosixShellsOnly(t *testing.T) {
	for _, sh := range []sandbox.Shell{
		{Kind: sandbox.ShellBash, Path: "bash"},
		{Kind: sandbox.ShellSh, Path: "sh"},
	} {
		if !strings.Contains(bash{shell: sh}.Description(), "python - <<'PY'") {
			t.Fatalf("%s description should carry the POSIX shell traps", sh.Kind)
		}
	}
	ps := bash{shell: sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: "powershell"}}
	if strings.Contains(ps.Description(), "python - <<'PY'") {
		t.Fatal("powershell description must not carry POSIX shell traps")
	}
}

// TestBashGitBashHintReachesToolOutput asserts the hint at its real boundary:
// the output ExecuteDetailed hands back to the model, with a negative control
// that the traceback alone is not enough.
func TestBashGitBashHintReachesToolOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("git-bash shim traps are windows-only")
	}
	sh := sandbox.ResolveShell("", "", nil)
	if !sh.Kind.IsPOSIX() {
		t.Skip("host has no POSIX shell")
	}
	b := bash{shell: sh, workDir: t.TempDir()}
	run := func(t *testing.T, command string) tool.DetailedResult {
		t.Helper()
		args, _ := json.Marshal(map[string]string{"command": command})
		res, err := b.ExecuteDetailed(context.Background(), args)
		if err != nil {
			t.Fatalf("command failed: %v (out=%q)", err, res.Output)
		}
		return res
	}
	mangled := `printf '%s\n' 'error: command exited: exit status 1' '  File "<string>", line 1' '    ||  goto :error' 'IndentationError: unexpected indent'`
	if out := run(t, mangled).Output; !strings.Contains(out, gitBashShimHint) {
		t.Fatalf("shim hint missing from tool output: %q", out)
	}
	plain := `printf '%s\n' '  File "<string>", line 1' 'IndentationError: unexpected indent'`
	if out := run(t, plain).Output; strings.Contains(out, gitBashShimHint) {
		t.Fatalf("hint appended without the shim trailer: %q", out)
	}
}
