package sandbox

import (
	"strings"
	"testing"
)

// The captured script must pin the text encodings and the exit-code
// normalization: a dropped clause silently restores CJK mojibake on reads,
// UTF-16LE on writes, or a flattened exit status.
func TestPowerShellCaptureScriptShape(t *testing.T) {
	script := powerShellCaptureScript("go test ./...")
	for _, want := range []string{
		"$PSDefaultParameterValues['Get-Content:Encoding']='utf8';",
		"$PSDefaultParameterValues['Out-File:Encoding']='utf8';",
		psUTF8Prologue,
		"go test ./...",
		"$__ok=$?;$__rc=$LASTEXITCODE;",
		"exit $__rc",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("captured script misses %q:\n%s", want, script)
		}
	}
	// Set-Content/Add-Content stay on PowerShell's ANSI default: their utf8 mode
	// prepends a BOM that breaks byte-exact writers (pid files, JSON).
	if strings.Contains(script, "Set-Content:Encoding") || strings.Contains(script, "Add-Content:Encoding") {
		t.Errorf("Set-Content/Add-Content must not be pinned to utf8:\n%s", script)
	}
	if !strings.HasPrefix(script, psCapturePrologue) {
		t.Errorf("captured script does not open with the capture prologue:\n%s", script)
	}
	if !strings.HasSuffix(script, psExitTrailer) {
		t.Errorf("captured script does not end with the exit trailer:\n%s", script)
	}
	if got := PowerShellUTF8Script("x"); got != psUTF8Prologue+"x" {
		t.Errorf("PowerShellUTF8Script = %q, want the bare UTF-8 prologue", got)
	}
}
