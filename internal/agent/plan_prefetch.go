package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// Host prefetch is the first form of the host-forced trigger: the host reads the
// paths the model itself named and hands the content back on the same round's
// tail. It is deliberately NOT a read receipt — the read coordinator is fed only
// by the agent's own read path, so a prefetched file still needs the model's own
// read before a write.
const (
	planPrefetchMaxPaths = 2
	planPrefetchTimeout  = 20 * time.Second
	planPrefetchMarker   = "[host prefetch — NOT a read receipt: a write to these paths still requires your own read_file]"
)

// planPrefetchTool is the reader the host runs. Its read-only profile is
// re-checked per call, exactly as parallel_calls checks its own nodes.
const planPrefetchTool = "read_file"

// planPrefetchPathPattern picks paths out of model prose: either a workspace
// path or a bare file name (the os.Stat filter below decides if it is real).
var planPrefetchPathPattern = regexp.MustCompile(`(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.(?:go|md|ts|tsx|js|jsx|json|toml|yaml|yml|py|rs|java|kt|sql|sh|css|html)`)

func (a *Agent) applyPlanPrefetch(ctx context.Context, calls []provider.ToolCall, outcomes []toolOutcome, receiptMark int) intervention {
	if a == nil || !a.prefetchEnabled() {
		return intervention{}
	}
	if !batchOutcomesAreReadOnly(outcomes) || !batchOutcomesSucceeded(outcomes) || !a.batchRoundMadeProgress(receiptMark) {
		return intervention{}
	}
	if !a.turn.loop.markPrefetched() {
		return intervention{}
	}
	sections := a.prefetchSections(ctx, calls)
	if len(sections) == 0 {
		return intervention{}
	}
	return intervention{verdict: verdictAdvise, guidance: planPrefetchMarker + "\n" + strings.Join(sections, "\n\n")}
}

// prefetchEnabled asks the registry rather than the configuration: the tier
// decides what boot registered, so the behaviour cannot drift from the surface.
func (a *Agent) prefetchEnabled() bool {
	if a.svc.tools == nil {
		return false
	}
	registered, ok := a.svc.tools.Get(tool.HostParallelCalls)
	if !ok {
		return false
	}
	hostCalls, ok := registered.(*ParallelCallsTool)
	return ok && hostCalls.Prefetch()
}

func (a *Agent) prefetchSections(ctx context.Context, calls []provider.ToolCall) []string {
	var sections []string
	for _, rel := range a.prefetchCandidates(calls) {
		body, err := a.readForPrefetch(ctx, rel)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		sections = append(sections, fmt.Sprintf("── %s ──\n%s", rel, body))
	}
	return sections
}

// prefetchCandidates are the workspace files the last assistant message named
// that this round is not already reading, capped so one round cannot flood the
// tail. Everything the model did not name is left alone: a statistical trigger
// would be guesswork.
func (a *Agent) prefetchCandidates(calls []provider.ToolCall) []string {
	busy := map[string]bool{}
	for _, call := range calls {
		for _, p := range planPrefetchPathPattern.FindAllString(call.Arguments, -1) {
			busy[prefetchKey(p)] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range planPrefetchPathPattern.FindAllString(a.sess.lastAssistantText(), -1) {
		rel := prefetchKey(p)
		if rel == "" || busy[rel] || seen[rel] {
			continue
		}
		info, err := os.Stat(a.prefetchPath(rel))
		if err != nil || info.IsDir() {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
		if len(out) >= planPrefetchMaxPaths {
			break
		}
	}
	return out
}

func prefetchKey(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(p)), "./")
}

func (a *Agent) prefetchPath(rel string) string {
	if root := strings.TrimSpace(a.writeWorkspaceRoot); root != "" {
		return filepath.Join(root, rel)
	}
	return rel
}

// readForPrefetch runs the reader through the registry without touching the read
// pipeline: no coverage, no envelope, no evidence for what it returns.
func (a *Agent) readForPrefetch(ctx context.Context, rel string) (string, error) {
	if a.svc.tools == nil {
		return "", fmt.Errorf("no tool registry")
	}
	reader, _, ambiguous := a.svc.tools.ResolveCall(planPrefetchTool)
	if reader == nil || len(ambiguous) > 0 {
		return "", fmt.Errorf("%s is not available", planPrefetchTool)
	}
	if !reader.ReadOnly() {
		return "", fmt.Errorf("%s is not statically read-only", planPrefetchTool)
	}
	args, err := json.Marshal(map[string]string{"path": rel})
	if err != nil {
		return "", err
	}
	readCtx, cancel := context.WithTimeout(ctx, planPrefetchTimeout)
	defer cancel()
	body, err := reader.Execute(readCtx, args)
	if err != nil {
		return "", err
	}
	body, _ = truncateToolOutputFor(body, planPrefetchTool, "host-prefetch:"+rel)
	return body, nil
}

// lastAssistantText reads the newest assistant content of the turn's session.
func (r *sessionRuntime) lastAssistantText() string {
	r.mu.Lock()
	session := r.conversation
	r.mu.Unlock()
	if session == nil {
		return ""
	}
	return session.lastAssistantText()
}

// lastAssistantText is the newest assistant content. Snapshot() copies every
// message, which is too heavy to call once per round.
func (s *Session) lastAssistantText() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, msg := range slices.Backward(s.Messages) {
		if msg.Role != provider.RoleAssistant {
			continue
		}
		if content := strings.TrimSpace(msg.Content); content != "" {
			return content
		}
	}
	return ""
}
