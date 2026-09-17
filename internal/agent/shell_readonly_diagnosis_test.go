package agent

import "testing"

// A shell command the host can prove read-only must survive the mutation
// barrier: diagnosing the failed change is the next thing the model needs, and
// skipping it costs a provider round. Verifications and writers stay skipped.
func TestShellCommandIsReadOnlyDiagnosis(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args string
		want bool
	}{
		{"proven read-only listing", "bash", `{"command":"ls -la"}`, true},
		{"proven read-only search", "bash", `{"command":"grep -r foo ."}`, true},
		{"proven read-only ripgrep", "bash", `{"command":"rg -n foo internal/agent"}`, true},
		{"powershell read-only inspection", "bash", `{"command":"Get-ChildItem -Path ."}`, true},
		{"proven read-only git status", "bash", `{"command":"git status --short"}`, true},
		{"verification still waits", "bash", `{"command":"go test ./..."}`, false},
		{"writer is not diagnosis", "bash", `{"command":"rm -rf build"}`, false},
		{"opaque command stays blocked", "bash", `{"command":"$(cat plan) --apply"}`, false},
		{"other tools are not shell diagnosis", "edit_file", `{"path":"a.go"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellCommandIsReadOnlyDiagnosis(tc.tool, tc.args); got != tc.want {
				t.Errorf("shellCommandIsReadOnlyDiagnosis(%q, %q) = %v, want %v", tc.tool, tc.args, got, tc.want)
			}
		})
	}
}
