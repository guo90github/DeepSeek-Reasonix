package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	// promptOptimizeChunkEvent carries one streamed text chunk: args (tabID, chunk).
	// promptOptimizeDoneEvent fires after the final chunk, args (tabID) — the
	// binding's resolved promise only means "no error"; the done event is the
	// completion signal so the frontend never races the async event queue.
	promptOptimizeChunkEvent = "prompt-optimize:chunk"
	promptOptimizeDoneEvent  = "prompt-optimize:done"
)

// OptimizePrompt rewrites the raw composer draft into a clearer instruction via
// the active session's configured optimize model, streaming each text chunk to
// the webview over promptOptimizeChunkEvent. It never touches the turn stream,
// session history, or the provider-visible prefix.
func (a *App) OptimizePrompt(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("输入为空，无法优化")
	}
	tab, ctrl := a.activeTabAndCtrl()
	if ctrl == nil {
		return "", fmt.Errorf("no active session")
	}
	tabID := ""
	if tab != nil {
		tabID = tab.ID
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	// Emit with a.ctx (not the request ctx): the async emitter flushes queued
	// events after this call returns, and a canceled request ctx would drop them.
	result, err := ctrl.OptimizePromptStream(ctx, text, func(chunk string) {
		a.runtimeEvents.Emit(a.ctx, promptOptimizeChunkEvent, tabID, chunk)
	})
	if err != nil {
		return "", err
	}
	a.runtimeEvents.Emit(a.ctx, promptOptimizeDoneEvent, tabID)
	return result, nil
}
