package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/draftoptimize"
	"reasonix/internal/provider"
)

const optimizeHistoryMaxTurnRunes = 500

// OptimizeDraft rewrites the composer draft with the standalone optimize model
// configured under [optimize] in the active workspace's config (project
// reasonix.toml or global config.toml). It is an independent, no-tool call that
// never writes to conversation history. auxContext carries bounded auxiliary
// metadata (referenced files, attachments, session titles) injected as DATA
// ONLY so the model can make the draft self-contained. When [optimize]
// include_history is on and the direction is all/contextual, the topic title
// and the last few user turns are also injected (DATA ONLY) so follow-up
// drafts that reference earlier turns stay rewriteable. Returns the rewritten
// draft, or an error when optimize isn't configured or the provider call fails.
func (a *App) OptimizeDraft(text, direction, auxContext string) (string, error) {
	tab, ctrl := a.activeTabAndCtrl()
	root := ""
	if tab != nil {
		root = tab.WorkspaceRoot
	}
	cfg, err := config.LoadForRoot(root)
	if err != nil {
		return "", err
	}
	opt := cfg.Optimize
	if !cfg.OptimizeEnabled() {
		return "", fmt.Errorf("输入优化未配置：请在 reasonix.toml 的 [optimize] 段设置 base_url、api_key、model")
	}
	aux := auxContext
	dir := draftoptimize.Direction(strings.TrimSpace(direction))
	if opt.IncludeHistory && opt.HistoryTurns > 0 && tab != nil && (dir == draftoptimize.All || dir == draftoptimize.Contextual) {
		if hist := a.optimizeHistoryAux(tab, ctrl, opt.HistoryTurns); hist != "" {
			aux = strings.TrimSpace(aux)
			if aux != "" {
				aux += "\n"
			}
			aux += hist
		}
	}
	timeout := time.Duration(opt.TimeoutMS) * time.Millisecond
	out, err := draftoptimize.Rewrite(context.Background(), draftoptimize.Options{
		BaseURL:   strings.TrimSpace(opt.BaseURL),
		APIKey:    config.ExpandVars(strings.TrimSpace(opt.APIKey)),
		Model:     strings.TrimSpace(opt.Model),
		MaxTokens: opt.MaxTokens,
		Timeout:   timeout,
	}, dir, text, aux)
	if err != nil {
		return "", err
	}
	return out, nil
}

// optimizeHistoryAux builds a bounded DATA-ONLY auxiliary block from the topic
// title and the most recent user-authored turns of the tab's session. It never
// includes assistant replies, so the smallest viable slice of prior context
// leaves the session.
func (a *App) optimizeHistoryAux(tab *WorkspaceTab, ctrl control.SessionAPI, turns int) string {
	parts := make([]string, 0, 3)
	if title := strings.TrimSpace(tab.TopicTitle); title != "" && !strings.EqualFold(title, "Untitled") {
		parts = append(parts, "Topic: "+title)
	}
	if ctrl != nil {
		users := recentUserTurnsFromMessages(ctrl.History())
		for i := len(users) - 1; i >= 0 && len(parts) < 1+turns; i-- {
			u := strings.TrimSpace(users[i])
			if runes := []rune(u); len(runes) > optimizeHistoryMaxTurnRunes {
				u = string(runes[:optimizeHistoryMaxTurnRunes])
			}
			if u == "" {
				continue
			}
			parts = append(parts, "Recent user turn: "+u)
		}
	}
	return strings.Join(parts, "\n")
}

// recentUserTurnsFromMessages extracts user-authored turns from the live
// in-memory controller history (the same filtering the topic-title path uses on
// the persisted session). Reading the controller rather than the session file
// avoids a stale snapshot: turns that have not yet flushed to the event log are
// still visible here, so the latest follow-up context is not dropped.
func recentUserTurnsFromMessages(msgs []provider.Message) []string {
	var users []string
	for _, m := range msgs {
		if m.Role != provider.RoleUser {
			continue
		}
		if !agent.IsUserAuthoredTurn(m.Content) {
			continue
		}
		content := control.StripComposePrefixes(agent.UserPreviewText(m.Content))
		content = control.StripReferencedContextPrefix(content)
		if strings.TrimSpace(content) != "" {
			users = append(users, content)
		}
	}
	return users
}
