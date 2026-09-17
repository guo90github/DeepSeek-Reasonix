//go:build windows

package sandbox

import (
	"errors"
	"os/exec"
	"testing"
)

// Windows PowerShell flattens every nonzero native exit code to 1, so without
// the trailer a no-match search and a real error both report 1; the captured
// script must surface the command's own status.
func TestPowerShellCapturedExitCode(t *testing.T) {
	sh := ResolveShell("powershell", "", nil)
	if sh.Kind != ShellPowerShell || sh.Path == "" {
		t.Skip("no Windows PowerShell interpreter on this host")
	}
	cases := []struct {
		command string
		want    int
	}{
		{"echo ok", 0},
		{"cmd /c exit 3", 3},
		{"exit 5", 5},
		{`cmd /c exit 7; "later statement succeeded"`, 0},
		{`Get-Content C:\__reasonix_missing__`, 1},
		{`Get-ChildItem C:\__reasonix_missing__; "later"`, 0},
		{`if ($true) { cmd /c exit 4 }`, 4},
		{"$t = @'\nhello\n'@\nWrite-Output \"[$t]\" # trailing comment", 0},
		{"echo a *>> $null", 0},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			argv := sh.argv(tc.command)
			err := exec.Command(argv[0], argv[1:]...).Run()
			got := 0
			var exitErr *exec.ExitError
			if err != nil {
				if !errors.As(err, &exitErr) {
					t.Fatalf("run %q: %v", tc.command, err)
				}
				got = exitErr.ExitCode()
			}
			if got != tc.want {
				t.Errorf("exit code for %q = %d, want %d", tc.command, got, tc.want)
			}
		})
	}
}
