package recap

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

type composeProvider struct {
	answer  string
	lastReq provider.Request
	lastSys string
}

func (p *composeProvider) Name() string { return "compose-fake" }

func (p *composeProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.lastReq = req
	if len(req.Messages) > 0 {
		p.lastSys = req.Messages[0].Content
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: p.answer}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{}}
	close(ch)
	return ch, nil
}

// A preview is a person waiting on one click, so it sends the prompt that belongs to
// that button, asks for no ceiling of its own, and reports which rules answered.
func TestComposeMemorySendsTheMemoryPromptWithoutACeiling(t *testing.T) {
	ctx := context.Background()
	fake := &composeProvider{answer: "以后提交一律用中文书写。"}
	got, err := ComposeMemory(ctx, fake, "fake/model", "note: 提交信息用中文", ComposeOptions{})
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if got.Text != "以后提交一律用中文书写。" {
		t.Fatalf("text = %q, want the model's answer", got.Text)
	}
	if got.PromptTag != MemoryPromptTag || got.Model != "fake/model" || got.Note != "" {
		t.Fatalf("result provenance = %+v, want the memory prompt's tag and the model ref", got)
	}
	if fake.lastSys != memoryPrompt {
		t.Fatal("the memory prompt must be the system message")
	}
	// -1 asks boundedllm for no ceiling of ours, which it forwards as 0: the model's
	// own output budget is the only ceiling left.
	if fake.lastReq.MaxTokens != 0 {
		t.Fatalf("MaxTokens = %d, want 0 (no ceiling of ours)", fake.lastReq.MaxTokens)
	}
}

func TestComposeSkillSendsTheSkillPromptAndUsesAnOverride(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := writePromptFile(t, dir, SkillPromptFileName, "Write the playbook from the notes."); err != nil {
		t.Fatal(err)
	}
	fake := &composeProvider{answer: "# Playbook\n\n## Steps\n1. do it"}
	got, err := ComposeSkill(ctx, fake, "fake/model", "notes", ComposeOptions{PromptDir: dir})
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if fake.lastSys != "Write the playbook from the notes." {
		t.Fatalf("override not sent: %q", fake.lastSys)
	}
	if !strings.HasPrefix(got.PromptTag, SkillPromptTag+"+") {
		t.Fatalf("tag = %q, want the skill prompt's fingerprinted tag", got.PromptTag)
	}
	if !strings.HasPrefix(got.Text, "# Playbook") {
		t.Fatalf("text = %q, want the model's playbook", got.Text)
	}
}

// Silence is reported rather than stored: the caller falls back and says why.
func TestComposeReportsAnEmptyAnswer(t *testing.T) {
	ctx := context.Background()
	fake := &composeProvider{answer: "   \n"}
	if _, err := ComposeMemory(ctx, fake, "fake/model", "note", ComposeOptions{}); err == nil {
		t.Fatal("an empty answer must be an error, not a memory")
	}
}
