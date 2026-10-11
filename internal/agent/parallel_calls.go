package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

// ParallelCallsTool runs concrete read-only tool calls the model already wrote
// out, concurrently, inside the round that asked for them. A sub-agent node
// would spend a model round of its own; these nodes spend only the host's wall
// clock, which is the whole point of the tier that registers this tool.
type ParallelCallsTool struct {
	reg *tool.Registry
	// prefetch moves the same opt-in one step further: the host also reads the
	// paths the model named, on the round tail.
	prefetch bool
}

// SetPrefetch arms the host-side prefetch. It is set from the tier at
// registration, so "tier ⇒ registration ⇒ behaviour" stays one chain.
func (p *ParallelCallsTool) SetPrefetch(on bool) { p.prefetch = on }

// Prefetch reports whether this run reads named paths for the model.
func (p *ParallelCallsTool) Prefetch() bool { return p.prefetch }

// NewParallelCallsTool builds the host-executed call surface. The registry is
// the only dependency: every node resolves through it and is rejected unless it
// declares itself statically read-only.
func NewParallelCallsTool(reg *tool.Registry) *ParallelCallsTool {
	return &ParallelCallsTool{reg: reg}
}

func (p *ParallelCallsTool) Name() string { return tool.HostParallelCalls }

func (p *ParallelCallsTool) Description() string {
	return "Run several concrete read-only tool calls at once and return every result in this one round. Use it when you already know the arguments of independent reads (files, greps, listings, LSP lookups): each node costs the host's wall clock but no extra model round. Nodes may only name statically read-only tools — bash and plugin tools are refused. For work that needs its own reasoning, use parallel_tasks instead."
}

func (p *ParallelCallsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "calls":{
    "type":"array",
    "description":"Independent read-only tool calls to run concurrently.",
    "items":{
      "type":"object",
      "properties":{
        "tool":{"type":"string","description":"Tool name; must be a statically read-only tool (read_file, grep, glob, ls, lsp_*, web_search, ...). bash and plugin tools are refused."},
        "arguments":{"type":"object","description":"The tool's argument object, exactly as you would pass it to that tool."},
        "description":{"type":"string","description":"Optional short label shown in the job list."},
        "timeout_seconds":{"type":"integer","minimum":1,"description":"Optional per-call timeout; defaults to 60s, capped at 300s."}
      },
      "required":["tool"]
    }
  }
},
"required":["calls"]
}`)
}

func (p *ParallelCallsTool) ReadOnly() bool { return true }

func (p *ParallelCallsTool) PlanModeSafe() bool { return true }

type parallelCallItem struct {
	Tool       string          `json:"tool"`
	Arguments  json.RawMessage `json:"arguments"`
	Description string         `json:"description"`
	TimeoutSec int             `json:"timeout_seconds"`
}

const (
	// parallelCallsMaxNodes bounds the request before any slice, channel or
	// goroutine exists: the input cap is the only guard that holds that early.
	parallelCallsMaxNodes = 32
	// parallelCallsMaxParallel mirrors the batch executor's own ceiling so a
	// host-executed batch cannot out-parallel the ordinary one.
	parallelCallsMaxParallel = 8
	parallelCallsMinTimeout  = time.Second
	parallelCallsDefault     = 60 * time.Second
	parallelCallsMaxTimeout  = 300 * time.Second
)

func (p *ParallelCallsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	parentID, sink, _, ok := CallContext(ctx)
	if !ok || sink == nil {
		parentID = tool.HostParallelCalls
		sink = event.Discard
	}
	calls, err := p.parseAndValidate(args)
	if err != nil {
		return "", err
	}

	n := len(calls)
	outputs := make([]string, n)
	errs := make([]error, n)
	doneCh := make(chan int, n)
	sem := make(chan struct{}, parallelCallsMaxParallel)
	var wg sync.WaitGroup

	for i := range calls {
		call := calls[i]
		call.Arguments = normalizeCallArgs(call.Arguments)
		subID := fmt.Sprintf("%s/call-%d", parentID, i+1)
		sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
			ID: subID, ParentID: parentID, Name: call.Tool, Args: string(call.Arguments), ReadOnly: true,
		}})
		wg.Go(func() {
			sem <- struct{}{}
			output, runErr := p.runNode(ctx, subID, sink, call)
			<-sem
			outputs[i], errs[i] = output, runErr
			if runErr != nil {
				sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
					ID: subID, ParentID: parentID, Name: call.Tool, Err: runErr.Error(),
				}})
			} else {
				sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
					ID: subID, ParentID: parentID, Name: call.Tool, Output: output, ReadOnly: true,
				}})
			}
			doneCh <- i
		})
	}

	settled := 0
	for settled < n {
		select {
		case <-doneCh:
			settled++
		case <-ctx.Done():
			wg.Wait()
			for i := range calls {
				if errs[i] == nil && outputs[i] == "" {
					errs[i] = ctx.Err()
				}
			}
			return formatParallelCallsAggregate(calls, outputs, errs), ctx.Err()
		}
	}
	wg.Wait()
	return formatParallelCallsAggregate(calls, outputs, errs), nil
}

// runNode resolves and runs one node. Only a tool that declares itself read-only
// is ever run — bash and plugin tools declare false by construction — so this
// path cannot carry a mutation, and therefore needs no write-side precondition.
func (p *ParallelCallsTool) runNode(ctx context.Context, subID string, sink event.Sink, call parallelCallItem) (string, error) {
	if p.reg == nil {
		return "", fmt.Errorf("%s is not configured", tool.HostParallelCalls)
	}
	resolved, _, ambiguous := p.reg.ResolveCall(call.Tool)
	if len(ambiguous) > 0 {
		return "", fmt.Errorf("ambiguous tool %q: %s", call.Tool, strings.Join(ambiguous, ", "))
	}
	if resolved == nil {
		return "", fmt.Errorf("unknown tool %q", call.Tool)
	}
	if !resolved.ReadOnly() {
		return "", fmt.Errorf("host-executed calls may only run statically read-only tools: %q can touch state", call.Tool)
	}
	nodeCtx, cancel := context.WithTimeout(withCallContext(ctx, subID, subSinkFor(subID, sink), nil, PlanModeFromContext(ctx)), callTimeout(call))
	defer cancel()
	output, err := resolved.Execute(nodeCtx, call.Arguments)
	output, _ = truncateToolOutputFor(output, call.Tool, subID)
	return output, err
}

func callTimeout(call parallelCallItem) time.Duration {
	timeout := parallelCallsDefault
	if call.TimeoutSec > 0 {
		timeout = time.Duration(call.TimeoutSec) * time.Second
	}
	return min(max(timeout, parallelCallsMinTimeout), parallelCallsMaxTimeout)
}

func normalizeCallArgs(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (p *ParallelCallsTool) parseAndValidate(args json.RawMessage) ([]parallelCallItem, error) {
	var params struct {
		Calls []parallelCallItem `json:"calls"`
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if len(params.Calls) == 0 {
		return nil, fmt.Errorf("at least one call is required")
	}
	if len(params.Calls) == 1 {
		return nil, fmt.Errorf("%s with a single call is that call; issue it directly instead", tool.HostParallelCalls)
	}
	if len(params.Calls) > parallelCallsMaxNodes {
		return nil, fmt.Errorf("%s accepts at most %d calls; got %d", tool.HostParallelCalls, parallelCallsMaxNodes, len(params.Calls))
	}
	for i, call := range params.Calls {
		if strings.TrimSpace(call.Tool) == "" {
			return nil, fmt.Errorf("call %d: tool is required", i+1)
		}
		if isSubagentToolName(call.Tool) {
			return nil, fmt.Errorf("call %d: %q runs its own worker; a host-executed node must be a concrete tool", i+1, call.Tool)
		}
		if strings.EqualFold(strings.TrimSpace(call.Tool), tool.HostParallelCalls) {
			return nil, fmt.Errorf("call %d: %s cannot nest itself", i+1, tool.HostParallelCalls)
		}
		if !jsonObjectArgs(call.Arguments) {
			return nil, fmt.Errorf("call %d: arguments must be a JSON object", i+1)
		}
	}
	return params.Calls, nil
}

func jsonObjectArgs(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || (trimmed[0] == '{' && json.Valid(trimmed))
}

func formatParallelCallsAggregate(calls []parallelCallItem, outputs []string, errs []error) string {
	items := make([]subagentAggregateItem, 0, len(calls))
	for i, call := range calls {
		item := subagentAggregateItem{header: fmt.Sprintf("── %s ──\n", callNodeLabel(call, i))}
		if errs[i] != nil {
			item.status = "status: failed\n"
			item.detail = fmt.Sprintf("[FAILED] %s\n", boundedInline(errs[i].Error(), 256))
		} else {
			item.status = "status: completed\n"
			item.answer = strings.TrimSpace(outputs[i])
		}
		items = append(items, item)
	}
	return formatBoundedSubagentAggregate(fmt.Sprintf("Completed %d host-executed calls:\n", len(calls)), items)
}

func callNodeLabel(call parallelCallItem, idx int) string {
	if label := strings.TrimSpace(call.Description); label != "" {
		return label
	}
	return fmt.Sprintf("%s-%d", call.Tool, idx+1)
}
