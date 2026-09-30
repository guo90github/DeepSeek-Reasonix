package main

import (
	"strings"
	"testing"
)

func TestHeartbeatWindowSpentOnlyNearTheCeiling(t *testing.T) {
	cases := []struct {
		name   string
		used   int
		window int
		want   bool
	}{
		{"a fresh session", 100, 100_000, false},
		{"three quarters full", 75_000, 100_000, false},
		{"just under the threshold", 95_000, 100_000, false},
		{"at the threshold", 96_000, 100_000, true},
		{"over the ceiling", 120_000, 100_000, true},
		{"unknown window", 5_000, 0, false},
		{"no usage reported", 0, 100_000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heartbeatWindowSpent(tc.used, tc.window); got != tc.want {
				t.Fatalf("spent(%d, %d) = %v, want %v", tc.used, tc.window, got, tc.want)
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
