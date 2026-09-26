package plugin

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"reasonix/internal/tool"
)

// capturingCallTransport records the params of the last tools/call, which is the
// only place an MCP server can learn who is calling it.
type capturingCallTransport struct {
	mu     sync.Mutex
	params map[string]any
}

func (t *capturingCallTransport) call(_ context.Context, method string, params any) (json.RawMessage, error) {
	if method != "tools/call" {
		return json.RawMessage(`{"tools":[{"name":"room_wait","description":"Wait for messages.","inputSchema":{"type":"object"}}]}`), nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if fields, ok := params.(map[string]any); ok {
		t.params = fields
	}
	return json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`), nil
}

func (t *capturingCallTransport) close() {}

func (t *capturingCallTransport) callMeta() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.params == nil {
		return nil
	}
	meta, _ := t.params["_meta"].(map[string]any)
	return meta
}

// One child serves every session of a plugin host, so a server that must address
// a wake at the session that asked reads the identity off the call. Without a
// caller the request stays untouched: the field is not a placeholder.
func TestToolsCallCarriesTheCallerSession(t *testing.T) {
	ctx := context.Background()
	tr := &capturingCallTransport{}
	c := &Client{name: "room", t: tr, spec: Spec{Name: "room"}, transport: "stdio"}
	tools, err := c.listTools(ctx)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}

	if _, err := tools[0].Execute(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if meta := tr.callMeta(); meta != nil {
		t.Fatalf("a call with no caller session carried _meta %v", meta)
	}

	callCtx := tool.WithCallerSessionPath(ctx, "/sessions/room.jsonl")
	if _, err := tools[0].Execute(callCtx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	meta := tr.callMeta()
	if got := meta[callerSessionMetaKey]; got != "/sessions/room.jsonl" {
		t.Fatalf("_meta[%s] = %v, want the caller's session", callerSessionMetaKey, got)
	}
	// The same identity routes a later wake: the child cannot be asked who a
	// notification is for, so the call that armed its server is the evidence.
	if got := c.wakeCaller(); got != "/sessions/room.jsonl" {
		t.Fatalf("wake route caller = %q, want the calling session", got)
	}
}

// A call with no caller session must not arm a wake route: a host serving
// several sessions would then deliver this session's mention to whichever one
// happens to be listed first.
func TestToolsCallWithoutCallerLeavesWakeRoutingEmpty(t *testing.T) {
	ctx := context.Background()
	tr := &capturingCallTransport{}
	c := &Client{name: "room", t: tr, spec: Spec{Name: "room"}, transport: "stdio"}
	tools, err := c.listTools(ctx)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if _, err := tools[0].Execute(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := c.wakeCaller(); got != "" {
		t.Fatalf("a callerless call armed wake routing to %q", got)
	}
}
