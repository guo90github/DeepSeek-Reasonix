package memory

import (
	"strings"
	"testing"
)

// The defect this gate set exists for: a tall Go task prompt pulled in a UI
// checklist that shares only body prose with it, because every common verb the
// merged facts mention looks rare in a store of ten facts.
func TestAutoRecallKeepsBodyOnlyOverlapOutOfALongTurn(t *testing.T) {
	store := recallTestStore(t)
	recallTestWrite(t, store.GlobalDir, Memory{
		ID: "mem-global-ui", Name: "ui-render-hard-requirements",
		Title:       "面板/卡片/表单渲染的硬性清单",
		Description: "任何面板/卡片/行/表单的渲染硬性清单（11 条）",
		Keywords:    "UI 渲染 面板 卡片 表单 徽标 版式 下拉 select 白底",
		Scope:       FactScopeGlobal, Type: TypeFeedback,
		Body: "面板落码前逐条自查：状态一律做成徽标 chip；空输入框必须有示例占位。" +
			"每次返工都是同一类原因，缺一项别提交。改动的每一条都要对得上屏上读数。",
	})
	recallTestWrite(t, store.Dir, Memory{
		ID: "mem-project-recall", Name: "memory-recall-prompt-and-cost",
		Title:       "记忆召回/提示词/成本与运行时闸门",
		Description: "记忆召回与提示词：召回预算、通用词过滤、成本公式",
		Keywords:    "记忆召回 提示词 预算 成本",
		Scope:       FactScopeProject, Type: TypeProject,
		Body: "自动召回必须控制预算并过滤泛化词。提示词优化走独立模型。",
	})

	long := strings.Repeat("这一轮先说明上下文：系统里用户提交的改动要一次落地，", 6) +
		"请分析 internal/control 的召回闸门并给出结论。"
	result := AutoRecall(store, long, RecallOptions{})
	for _, hit := range result.Hits {
		if hit.Memory.ID == "mem-global-ui" {
			t.Fatalf("body-only overlap pulled the UI checklist into a Go turn: %+v", result.Hits)
		}
	}
	if len(result.Hits) == 0 && result.Suppressed == "" {
		t.Fatalf("a silent turn must say which gate turned it away: %+v", result)
	}

	pointed := AutoRecall(store, "记忆召回的提示词优化为什么要控制预算", RecallOptions{})
	if len(pointed.Hits) == 0 || pointed.Hits[0].Memory.ID != "mem-project-recall" {
		t.Fatalf("a pointed reference to the fact's own topic was lost: %+v", pointed)
	}
	if !strings.Contains(pointed.Hits[0].Reason, "label fields") {
		t.Fatalf("reason must name where the evidence was found, got %q", pointed.Hits[0].Reason)
	}
}

// Two bigrams of one three-character word are one shared word, not two matches.
func TestAutoRecallNeedsAPhraseNotAWordPair(t *testing.T) {
	store := recallTestStore(t)
	recallTestWrite(t, store.Dir, Memory{
		ID: "mem-login", Name: "login-notes", Title: "登录相关",
		Description: "登录相关的记录", Type: TypeProject,
		Scope: FactScopeProject, Body: "登录入口在设置页。",
	})
	if result := AutoRecall(store, "登录模块", RecallOptions{}); len(result.Hits) != 0 {
		t.Fatalf("one shared two-character word must not recall: %+v", result.Hits)
	}
}
