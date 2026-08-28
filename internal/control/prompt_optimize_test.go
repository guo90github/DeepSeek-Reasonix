package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type promptOptimizeTestProvider struct {
	lastRequest provider.Request
}

func (p *promptOptimizeTestProvider) Name() string { return "optimize-test" }

func (p *promptOptimizeTestProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.lastRequest = req
	out := make(chan provider.Chunk, 2)
	go func() {
		defer close(out)
		select {
		case out <- provider.Chunk{Type: provider.ChunkText, Text: "请修复登录接口的鉴权漏洞。"}:
		case <-ctx.Done():
			return
		}
		out <- provider.Chunk{Type: provider.ChunkDone}
	}()
	return out, nil
}

func optimizeTestController(t *testing.T, stub *promptOptimizeTestProvider, session *agent.Session) (*Controller, *string) {
	t.Helper()
	var resolvedRef string
	c := &Controller{
		// The session model must never be the optimizer: only the dedicated
		// promptOptimizeModel ref is resolved.
		modelRef:            "session/model",
		promptOptimizeModel: "opt/qwen",
		promptOptimizeProviderResolver: func(ref string) (provider.Provider, error) {
			resolvedRef = ref
			return stub, nil
		},
		sink: event.Discard,
	}
	if session != nil {
		c.executor = agent.New(nil, nil, session, agent.Options{}, event.Discard)
	}
	return c, &resolvedRef
}

func TestOptimizePromptStreamsThroughDedicatedModel(t *testing.T) {
	stub := &promptOptimizeTestProvider{}
	c, resolvedRef := optimizeTestController(t, stub, nil)
	got, err := c.OptimizePrompt(context.Background(), "帮我改个 bug")
	if err != nil {
		t.Fatalf("OptimizePrompt: %v", err)
	}
	if !strings.Contains(got, "鉴权漏洞") {
		t.Fatalf("optimized = %q", got)
	}
	if *resolvedRef != "opt/qwen" {
		t.Fatalf("resolver ref = %q, want the dedicated model, not the session model", *resolvedRef)
	}
	if len(stub.lastRequest.Messages) != 2 || stub.lastRequest.Messages[0].Role != provider.RoleSystem {
		t.Fatalf("request messages = %+v", stub.lastRequest.Messages)
	}
	if strings.Contains(stub.lastRequest.Messages[1].Content, "最近会话上下文") {
		t.Fatalf("empty session must not inject a context block: %q", stub.lastRequest.Messages[1].Content)
	}
}

func TestOptimizePromptIncludesRecentSessionContext(t *testing.T) {
	session := agent.NewSession("system")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "我要重构登录模块"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "好的，请说明具体问题。"})
	stub := &promptOptimizeTestProvider{}
	c, _ := optimizeTestController(t, stub, session)
	c.goals.goal = "重构登录模块"
	if _, err := c.OptimizePrompt(context.Background(), "帮我改个 bug"); err != nil {
		t.Fatalf("OptimizePrompt: %v", err)
	}
	userContent := stub.lastRequest.Messages[1].Content
	for _, want := range []string{"最近会话上下文", "我要重构登录模块", "当前目标：重构登录模块", "待优化提示词"} {
		if !strings.Contains(userContent, want) {
			t.Fatalf("user content missing %q: %q", want, userContent)
		}
	}
}

func TestOptimizePromptRejectsEmptyInputAndUnconfiguredModel(t *testing.T) {
	stub := &promptOptimizeTestProvider{}
	c, _ := optimizeTestController(t, stub, nil)
	if _, err := c.OptimizePrompt(context.Background(), "   "); err == nil {
		t.Fatal("expected empty-input error")
	}
	unconfigured := &Controller{modelRef: "session/model"}
	if _, err := unconfigured.OptimizePrompt(context.Background(), "hi"); err == nil {
		t.Fatal("expected unconfigured-model error")
	}
	noResolver := &Controller{promptOptimizeModel: "opt/qwen"}
	if _, err := noResolver.OptimizePrompt(context.Background(), "hi"); err == nil {
		t.Fatal("expected missing-resolver error")
	}
}
