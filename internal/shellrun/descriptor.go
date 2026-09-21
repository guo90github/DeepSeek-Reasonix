package shellrun

import (
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

// DescriptorFromShell builds a partial ShellExecution from a resolved sandbox
// Shell. It fills identity fields only; callers set state/phase after the run.
func DescriptorFromShell(sh sandbox.Shell) *tool.ShellExecution {
	ex := &tool.ShellExecution{
		Kind:           "shell",
		Platform:       platformName(),
		SupportsAndAnd: sh.SupportsChaining(),
	}
	name, version := classifyShell(sh)
	ex.Shell = name
	if version != "" {
		ex.ShellVersion = version
	}
	return ex
}

// DisplayName returns the label a shell invocation shows in cards and CLI lines.
// It is interpreter-independent on purpose: the interpreter stays in execution
// metadata, so no header can be read as a command that never ran.
func DisplayName() string {
	return tool.ShellDisplayLabel
}

// classifyShell maps a resolved Shell to contract names.
// powershell.exe → powershell / 5.1; pwsh → pwsh / 7+; Git for Windows bash → git-bash.
func classifyShell(sh sandbox.Shell) (name, version string) {
	base := strings.ToLower(filepath.Base(sh.Path))
	base = strings.TrimSuffix(base, ".exe")
	switch sh.Kind {
	case sandbox.ShellPowerShell:
		if base == "pwsh" || sh.SupportsChaining() {
			return tool.ShellNamePwsh, tool.ShellVersionPS7
		}
		return tool.ShellNamePowerShell, tool.ShellVersionPS51
	case sandbox.ShellZsh:
		return tool.ShellNameZsh, ""
	case sandbox.ShellSh:
		return tool.ShellNameSh, ""
	default:
		if isGitBashPath(sh.Path) {
			return tool.ShellNameGitBash, ""
		}
		return tool.ShellNameBash, ""
	}
}

func isGitBashPath(path string) bool {
	if path == "" {
		return false
	}
	// Normalize separators so both "Git\bin\bash.exe" and "Git/bin/bash" match.
	norm := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	return strings.Contains(norm, "/git/") && strings.Contains(norm, "bash")
}

func platformName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "darwin"
	default:
		return "linux"
	}
}
