package agent

import "testing"

// TestExceededNamesTheTokenAxis pins the axis name the Goal stop-cause mapping keys
// on: the budget gate reports "token" (singular), and an unconfigured token limit
// stops nothing — the same three states the cost and time axes are tested in.
func TestExceededNamesTheTokenAxis(t *testing.T) {
	cases := []struct {
		name     string
		budget   runBudget
		limit    TaskBudget
		wantAxis string
	}{
		{
			"inside the token budget",
			runBudget{promptTokens: 6, outputTokens: 5},
			TaskBudget{Tokens: 20},
			"",
		},
		{
			"past the token budget",
			runBudget{promptTokens: 6, outputTokens: 5},
			TaskBudget{Tokens: 10},
			"token",
		},
		{
			"an unconfigured token limit stops nothing",
			runBudget{promptTokens: 999, outputTokens: 999},
			TaskBudget{},
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			budget := tc.budget
			axis, detail := budget.exceeded(tc.limit)
			if axis != tc.wantAxis {
				t.Fatalf("axis = %q (%s), want %q", axis, detail, tc.wantAxis)
			}
			if tc.wantAxis != "" && detail == "" {
				t.Fatal("a crossing must say what it crossed")
			}
		})
	}
}
