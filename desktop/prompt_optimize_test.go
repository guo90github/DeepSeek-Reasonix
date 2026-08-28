package main

import (
	"sync"
	"testing"
	"time"
)

func collectChunkEmissions() (func(chunk string), *[]string, *sync.Mutex) {
	var mu sync.Mutex
	var emissions []string
	return func(chunk string) {
		mu.Lock()
		defer mu.Unlock()
		emissions = append(emissions, chunk)
	}, &emissions, &mu
}

func TestChunkFlusherFlushNowEmitsCombinedInOrder(t *testing.T) {
	emit, emissions, mu := collectChunkEmissions()
	f := newChunkFlusher(emit)
	f.push("he")
	f.push("llo")
	f.flushNow()
	mu.Lock()
	defer mu.Unlock()
	if len(*emissions) != 1 || (*emissions)[0] != "hello" {
		t.Fatalf("want one combined emission %q, got %q", "hello", *emissions)
	}
}

func TestChunkFlusherCoalescesOnInterval(t *testing.T) {
	emit, emissions, mu := collectChunkEmissions()
	f := newChunkFlusher(emit)
	f.push("a")
	f.push("b")
	time.Sleep(promptOptimizeFlushInterval + 30*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(*emissions) != 1 || (*emissions)[0] != "ab" {
		t.Fatalf("want one interval emission %q, got %q", "ab", *emissions)
	}
}

func TestChunkFlusherFlushNowStopsPendingTimer(t *testing.T) {
	emit, emissions, mu := collectChunkEmissions()
	f := newChunkFlusher(emit)
	f.push("x")
	f.flushNow()
	time.Sleep(promptOptimizeFlushInterval + 30*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(*emissions) != 1 {
		t.Fatalf("flushNow must stop the pending timer, got %d emissions", len(*emissions))
	}
}

func TestChunkFlusherEmptyFlushEmitsNothing(t *testing.T) {
	emit, emissions, mu := collectChunkEmissions()
	f := newChunkFlusher(emit)
	f.flushNow()
	mu.Lock()
	defer mu.Unlock()
	if len(*emissions) != 0 {
		t.Fatalf("empty flush must not emit, got %q", *emissions)
	}
}
