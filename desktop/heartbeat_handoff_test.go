package main

import (
	"strings"
	"testing"

	"reasonix/internal/control"
)

// heartbeatUsageStub is the read-only usage half of a controller.
type heartbeatUsageStub struct {
	used   int
	window int
}

func (s heartbeatUsageStub) RuntimeStatus() control.RuntimeStatus { return control.RuntimeStatus{} }

func (s heartbeatUsageStub) ContextSnapshot() (int, int) { return s.used, s.window }

// heartbeatContextStub reports both facts a real controller exposes: the numbers
// and the provider's own verdict on the window.
type heartbeatContextStub struct {
	used      int
	window    int
	exhausted bool
}

func (s heartbeatContextStub) RuntimeStatus() control.RuntimeStatus { return control.RuntimeStatus{} }

func (s heartbeatContextStub) ContextSnapshot() (int, int) { return s.used, s.window }

func (s heartbeatContextStub) ContextExhausted() bool { return s.exhausted }

func TestHeartbeatSpentWindowFollowsTheSwitchAndIgnoresTheGoal(t *testing.T) {
	cases := []struct {
		name       string
		ctrl       heartbeatRuntimeStatus
		unattended bool
		percent    int
		want       bool
	}{
		{"a spent window while unattended", heartbeatUsageStub{used: 90_000, window: 100_000}, true, 0, true},
		{"a spent window with the switch off", heartbeatUsageStub{used: 90_000, window: 100_000}, false, 0, false},
		{"a window with room left", heartbeatUsageStub{used: 10_000, window: 100_000}, true, 0, false},
		{"a configured threshold that is not reached", heartbeatUsageStub{used: 60_000, window: 100_000}, true, 70, false},
		{"a configured threshold that is reached", heartbeatUsageStub{used: 60_000, window: 100_000}, true, 55, true},
		{"a session that died on the window", heartbeatContextStub{exhausted: true}, true, 0, true},
		{"a session that died on the window with the switch off", heartbeatContextStub{exhausted: true}, false, 0, false},
		{"a controller that reports no usage", &heartbeatGoalCtrlStub{}, true, 0, false},
		{"no controller", nil, true, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heartbeatSpentWindow(tc.ctrl, tc.unattended, tc.percent); got != tc.want {
				t.Fatalf("heartbeatSpentWindow = %v, want %v", got, tc.want)
			}
		})
	}
}

// The provider's own answer outranks the numbers: a session that already failed
// on the window is spent even when no window size was ever reported.
func TestHeartbeatSessionSpentWithoutAnyWindowNumbers(t *testing.T) {
	ctrl := heartbeatContextStub{exhausted: true}
	if _, window := heartbeatContextNumbers(ctrl); window != 0 {
		t.Fatalf("the stub must report no window, got %d", window)
	}
	if !heartbeatSessionSpent(ctrl, unattendedHandoffPercent) {
		t.Fatal("a session whose turn died on the window must be spent")
	}
	if heartbeatSessionSpent(heartbeatContextStub{used: 1_000, window: 100_000}, unattendedHandoffPercent) {
		t.Fatal("a session with room left and no failure must not be spent")
	}
}

func TestHeartbeatWindowSpentOnlyNearTheCeiling(t *testing.T) {
	cases := []struct {
		name    string
		used    int
		window  int
		percent int
		want    bool
	}{
		{"a fresh session", 100, 100_000, 0, false},
		{"half full", 50_000, 100_000, 0, false},
		{"just under the default", 89_000, 100_000, 0, false},
		{"at the default", 90_000, 100_000, 0, true},
		{"over the ceiling", 120_000, 100_000, 0, true},
		{"a lower configured threshold fires earlier", 60_000, 100_000, 55, true},
		{"a higher configured threshold waits", 92_000, 100_000, 95, false},
		{"a nonsense threshold falls back to the default", 70_000, 100_000, 900, false},
		{"unknown window", 5_000, 0, 0, false},
		{"no usage reported", 0, 100_000, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heartbeatWindowSpent(tc.used, tc.window, tc.percent); got != tc.want {
				t.Fatalf("spent(%d, %d, %d) = %v, want %v", tc.used, tc.window, tc.percent, got, tc.want)
			}
		})
	}
}

func TestHeartbeatHandoffPercentNormalizesIntoRange(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"unset", 0, unattendedHandoffPercent},
		{"negative", -5, unattendedHandoffPercent},
		{"in range", 75, 75},
		{"at the floor", minUnattendedHandoffPercent, minUnattendedHandoffPercent},
		{"at the ceiling", maxUnattendedHandoffPercent, maxUnattendedHandoffPercent},
		{"below the floor is clamped", 10, minUnattendedHandoffPercent},
		{"above the ceiling is clamped", 900, maxUnattendedHandoffPercent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeUnattendedHandoffPercent(tc.in); got != tc.want {
				t.Fatalf("normalize(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestHeartbeatHandoffPrefaceCarriesTheContractAndTheOldTranscript(t *testing.T) {
	task := HeartbeatTask{Goal: "ship the unattended driver"}
	preface := heartbeatHandoffPreface(task, "/state/sessions/old.jsonl")
	for _, want := range []string{"接续会话", "ship the unattended driver", "/state/sessions/old.jsonl"} {
		if !strings.Contains(preface, want) {
			t.Fatalf("preface %q is missing %q", preface, want)
		}
	}
}

func TestHeartbeatHandoffPrefaceStaysUsableWithoutAPath(t *testing.T) {
	preface := heartbeatHandoffPreface(HeartbeatTask{}, "")
	if !strings.Contains(preface, "接续会话") {
		t.Fatalf("preface %q must still announce the handoff", preface)
	}
	if strings.Contains(preface, "上一会话的完整记录在") {
		t.Fatalf("preface %q must not claim a transcript path it does not have", preface)
	}
}

// The session left behind must say where its task went.
func TestHeartbeatHandoffNoticeNamesTheNewSession(t *testing.T) {
	notice := heartbeatHandoffNotice("Heartbeat: 无人值守推进器")
	for _, want := range []string{"上下文窗口已用尽", "已交接给新会话", "Heartbeat: 无人值守推进器", "本会话不再推进"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice %q is missing %q", notice, want)
		}
	}
}

func TestHeartbeatHandoffPromptIsOneShot(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	engine.handoffPreface = map[string]string{"t1": "preface"}
	if got := engine.takeHandoffPrompt(HeartbeatTask{ID: "t1", Prompt: "work"}); got != "preface\n\nwork" {
		t.Fatalf("first submission = %q, want the preface then the prompt", got)
	}
	if got := engine.takeHandoffPrompt(HeartbeatTask{ID: "t1", Prompt: "work"}); got != "work" {
		t.Fatalf("second submission = %q, want the plain prompt", got)
	}
	if got := engine.takeHandoffPrompt(HeartbeatTask{ID: "other", Prompt: "work"}); got != "work" {
		t.Fatalf("unrelated task = %q, want the plain prompt", got)
	}
}

func TestHeartbeatHandoffPromptWithoutAPrefaceIsThePlainPrompt(t *testing.T) {
	engine := newHeartbeatEngine(nil)
	if got := engine.takeHandoffPrompt(HeartbeatTask{ID: "t1", Prompt: "work"}); got != "work" {
		t.Fatalf("prompt = %q, want the plain prompt", got)
	}
}
