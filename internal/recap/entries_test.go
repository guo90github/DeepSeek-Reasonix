package recap

import (
	"strings"
	"testing"
)

// One button group per restatement is a cost the reader pays for nothing: a long
// session that states the same decision twice must offer it once, with both
// pointers kept.
func TestParseEntriesMergesRestatementsOfOneTopic(t *testing.T) {
	entries, ok := parseEntries(`[
	  {"kind":"fact","body":"提交信息一律用中文书写，commit message 不要写英文","evidence":"desktop/AGENTS.md"},
	  {"kind":"fact","body":"commit message 必须用中文写，别写英文","evidence":"memory:提交信息"},
	  {"kind":"handoff","body":"打包脚本还没验过 verify-windows-portable.sh"}
	]`)
	if !ok {
		t.Fatal("parseEntries refused a valid array")
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (the commit-message restatement merged): %+v", len(entries), entries)
	}
	if !strings.Contains(entries[0].Evidence, "AGENTS.md") || !strings.Contains(entries[0].Evidence, "memory:提交信息") {
		t.Fatalf("merged evidence = %q, want both restatements' evidence", entries[0].Evidence)
	}
}

func TestTopicTokensKeepDifferentTopicsApart(t *testing.T) {
	same := [][2]string{
		{"提交信息一律用中文书写，commit message 不要写英文", "commit message 必须用中文写，别写英文"},
		{"打包时不要用 PowerShell 回写仓库文件", "别用 PowerShell 覆盖仓库里的文件"},
	}
	for _, pair := range same {
		if !sameTopicTokens(topicTokens(pair[0]), topicTokens(pair[1])) {
			t.Fatalf("%q and %q are the same topic", pair[0], pair[1])
		}
	}
	different := [][2]string{
		{"提交信息一律用中文书写，commit message 不要写英文", "打包时不要用 PowerShell 回写仓库文件"},
		{"回顾页的按钮太多", "把 token 预算改成不设限"},
	}
	for _, pair := range different {
		if sameTopicTokens(topicTokens(pair[0]), topicTokens(pair[1])) {
			t.Fatalf("%q and %q are different topics", pair[0], pair[1])
		}
	}
}
