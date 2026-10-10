package agent

import (
	"strings"
	"testing"
	"time"
)

// TestTurnMetricsWorthReporting pins when the readout appears: a turn that took
// several rounds or real wall clock, never a quick one.
func TestTurnMetricsWorthReporting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rounds int
		wall   time.Duration
		want   bool
	}{
		{name: "one quick round stays quiet", rounds: 1, wall: 3 * time.Second, want: false},
		{name: "just under both thresholds", rounds: turnMetricsMinRounds - 1, wall: turnMetricsMinWall - time.Second, want: false},
		{name: "enough rounds", rounds: turnMetricsMinRounds, wall: 2 * time.Second, want: true},
		{name: "enough wall clock", rounds: 1, wall: turnMetricsMinWall, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := turnMetricsWorthReporting(tc.rounds, tc.wall); got != tc.want {
				t.Fatalf("worth reporting = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTurnMetricsDetailSplitsTheWallClock keeps the numbers honest: model time is
// whatever the tool windows left over, and a bogus tool total cannot make it
// negative.
func TestTurnMetricsDetailSplitsTheWallClock(t *testing.T) {
	got := turnMetricsDetail(7, 100*time.Second, 20*time.Second)
	for _, want := range []string{"rounds=7", "wall=1m40s", "model=1m20s (80%)", "tools=20s (20%)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail = %q, want it to contain %q", got, want)
		}
	}

	clamped := turnMetricsDetail(5, 10*time.Second, 40*time.Second)
	if !strings.Contains(clamped, "model=0s (0%)") {
		t.Fatalf("a tool total past the wall clock did not clamp: %q", clamped)
	}
}
