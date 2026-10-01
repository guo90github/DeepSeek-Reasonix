package main

import (
	"strconv"
	"strings"
	"sync"

	"reasonix/internal/agent"
	"reasonix/internal/evidence"
)

// S2 (docs/40 §5 step 3): a restored tab with no bound controller still shows
// what it owes, replayed from its transcript. Reading a whole transcript is too
// expensive to repeat, so results are memoized by (path, digest, revision).

type todoReplayResult struct {
	todos []evidence.TodoItem
	board agent.TodoBoard
	has   bool
}

type todoReplayCache struct {
	mu      sync.Mutex
	entries map[string]todoReplayResult
}

const todoReplayCacheLimit = 16

var todoHistoryReplays = &todoReplayCache{entries: map[string]todoReplayResult{}}

func (c *todoReplayCache) get(key string) (todoReplayResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	res, ok := c.entries[key]
	return res, ok
}

func (c *todoReplayCache) put(key string, res todoReplayResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= todoReplayCacheLimit {
		c.entries = map[string]todoReplayResult{}
	}
	c.entries[key] = res
}

func resetTodoHistoryReplays() {
	todoHistoryReplays.mu.Lock()
	defer todoHistoryReplays.mu.Unlock()
	todoHistoryReplays.entries = map[string]todoReplayResult{}
}

// replayedTodosForSession reports the task list and board a transcript carries.
// has is false when the session has never recorded a task list, which is what
// keeps an empty tab from claiming it owes nothing.
func replayedTodosForSession(sessionPath, digest string, revision int64) ([]evidence.TodoItem, agent.TodoBoard, bool) {
	if strings.TrimSpace(sessionPath) == "" {
		return nil, agent.TodoBoard{}, false
	}
	key := sessionPath + "|" + digest + "|" + strconv.FormatInt(revision, 10)
	if res, ok := todoHistoryReplays.get(key); ok {
		return res.todos, res.board, res.has
	}
	res := todoReplayResult{}
	if loaded, err := agent.LoadSession(sessionPath); err == nil && loaded != nil {
		todos, board, has := agent.ReplayTodoHistory(loaded.Messages)
		res = todoReplayResult{todos: todos, board: board, has: has}
	}
	todoHistoryReplays.put(key, res)
	return res.todos, res.board, res.has
}
