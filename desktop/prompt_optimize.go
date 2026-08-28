package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// promptOptimizeChunkEvent carries one streamed text chunk: args (tabID, chunk).
	// promptOptimizeDoneEvent fires after the final chunk, args (tabID) — the
	// binding's resolved promise only means "no error"; the done event is the
	// completion signal so the frontend never races the async event queue.
	promptOptimizeChunkEvent = "prompt-optimize:chunk"
	promptOptimizeDoneEvent  = "prompt-optimize:done"
	// promptOptimizeFlushInterval bounds the chunk-event cadence: a fast
	// provider stream emits one event per interval instead of one Wails IPC
	// round trip per token.
	promptOptimizeFlushInterval = 50 * time.Millisecond
)

// chunkFlusher coalesces streamed chunks into one emit per flush interval so a
// fast token stream cannot flood the webview IPC channel per chunk. flushNow
// must run before the done event so the final batch is not lost.
type chunkFlusher struct {
	emit  func(chunk string)
	mu    sync.Mutex
	buf   strings.Builder
	timer *time.Timer
}

func newChunkFlusher(emit func(chunk string)) *chunkFlusher {
	return &chunkFlusher{emit: emit}
}

// push appends a chunk and arms the flush timer on the first chunk of a batch.
func (f *chunkFlusher) push(chunk string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf.WriteString(chunk)
	if f.timer == nil {
		f.timer = time.AfterFunc(promptOptimizeFlushInterval, f.flush)
	}
}

// flush emits whatever accumulated since the last flush. It runs on the timer
// goroutine or the stream goroutine — the mutex keeps them mutually exclusive.
func (f *chunkFlusher) flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushLocked()
}

// flushNow stops the pending timer and emits the remaining buffer.
func (f *chunkFlusher) flushNow() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushLocked()
}

func (f *chunkFlusher) flushLocked() {
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	if f.buf.Len() == 0 {
		return
	}
	combined := f.buf.String()
	f.buf.Reset()
	f.emit(combined)
}

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
	flusher := newChunkFlusher(func(chunk string) {
		a.runtimeEvents.Emit(a.ctx, promptOptimizeChunkEvent, tabID, chunk)
	})
	result, err := ctrl.OptimizePromptStream(ctx, text, flusher.push)
	if err != nil {
		return "", err
	}
	flusher.flushNow()
	a.runtimeEvents.Emit(a.ctx, promptOptimizeDoneEvent, tabID)
	return result, nil
}
