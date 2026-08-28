package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// OptimizePrompt rewrites the raw composer draft into a clearer instruction via
// the active session's configured model. It never touches the turn stream,
// session history, or the provider-visible prefix.
func (a *App) OptimizePrompt(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("输入为空，无法优化")
	}
	_, ctrl := a.activeTabAndCtrl()
	if ctrl == nil {
		return "", fmt.Errorf("no active session")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return ctrl.OptimizePrompt(ctx, text)
}
