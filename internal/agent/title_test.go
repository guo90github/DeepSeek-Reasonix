package agent

import (
	"testing"

	"reasonix/internal/provider"
)

func TestReasoningLanguageDirectiveIsNotUserAuthored(t *testing.T) {
	for _, injected := range []string{
		"<reasoning-language>\nUse Simplified Chinese",
		"<reasoning-language>\nUse Simplified Chinese\n</reasoning-language>",
	} {
		if IsUserAuthoredTurnMessage(provider.Message{Role: provider.RoleUser, Content: injected}) {
			t.Fatalf("reasoning-language directive %q must not count as user-authored", injected)
		}
	}
}

func TestFinishProtocolRepairIsNotUserAuthored(t *testing.T) {
	for _, injected := range []string{
		"Protocol repair: finish this turn now. A visible final answer has already been provided, so do not repeat it. Call finish exactly once as the only tool call with outcome completed, partial, or blocked.",
		"Protocol repair: finish this turn now. The finish call has already been accepted, so do not call it again. Provide the visible final answer now.",
		"Protocol repair: finish this turn now. Provide the visible final answer and call finish exactly once as the only tool call. If you need the user's answer instead, call ask and do not call finish.",
		// withTurnPreferences wraps the injected prompt in transient blocks;
		// the persisted shape must still classify as synthetic.
		"<reasoning-language>\nzh\n</reasoning-language>\n\n<response-language>\nzh\n</response-language>\n\nProtocol repair: finish this turn now. A visible final answer has already been provided, so do not repeat it.",
	} {
		if IsUserAuthoredTurn(injected) {
			t.Fatalf("finish protocol repair %q must not count as user-authored", injected)
		}
	}
}

func TestStripPasteDisplayLabel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simplified Chinese", in: "[已粘贴文本 #1 · 100 行]\npackage main", want: "package main"},
		{name: "traditional Chinese", in: "[已貼上文字 #2 · 5 行]\r\n帮我看看这段代码", want: "帮我看看这段代码"},
		{name: "English", in: "[Pasted text #3 · 42 lines]\nfunc foo() {}", want: "func foo() {}"},
		{name: "inline mention", in: "Explain [Pasted text #1 · 2 lines] handling", want: "Explain [Pasted text #1 · 2 lines] handling"},
		{name: "later standalone line", in: "Keep this\n[Pasted text #1 · 2 lines]\nverbatim", want: "Keep this\n[Pasted text #1 · 2 lines]\nverbatim"},
		{name: "no label", in: "debug the login issue", want: "debug the login issue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripPasteDisplayLabel(tt.in); got != tt.want {
				t.Fatalf("StripPasteDisplayLabel(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
