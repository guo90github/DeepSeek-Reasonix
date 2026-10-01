package agent

import (
	"strconv"
	"strings"

	"reasonix/internal/evidence"
)

// TodoBoard is what the task shelf shows: the unfinished queue across every list
// this session has carried, plus a read-only archive of what finished. A later
// todo_write no longer erases work an earlier list left unfinished — that work
// joins the queue and stays visible (requirement 17).
type TodoBoard struct {
	Queue   []evidence.TodoItem `json:"queue,omitempty"`
	Archive []evidence.TodoItem `json:"archive,omitempty"`
}

// todoBoardLimit bounds each side. The board is a shelf, not a ledger: the
// oldest entries fall off rather than growing without end.
const todoBoardLimit = 200

// MergeTodoBoard folds one incoming list into the board:
//   - an item in the list is upserted (its status is the newest word);
//   - finished items leave the queue and enter the archive;
//   - queue entries the list never mentions stay put, so a re-plan cannot make
//     unfinished work invisible.
func MergeTodoBoard(board TodoBoard, incoming []evidence.TodoItem) TodoBoard {
	if len(incoming) == 0 {
		return board
	}
	queue := make([]evidence.TodoItem, 0, len(board.Queue)+len(incoming))
	index := make(map[string]int, len(board.Queue))
	for _, item := range board.Queue {
		if key := todoBoardKey(item); key != "" {
			if _, seen := index[key]; seen {
				continue
			}
			index[key] = len(queue)
		}
		queue = append(queue, item)
	}
	archive := append([]evidence.TodoItem(nil), board.Archive...)
	archived := make(map[string]struct{}, len(archive))
	for _, item := range archive {
		archived[todoBoardKey(item)] = struct{}{}
	}

	finished := make(map[string]struct{}, len(incoming))
	for _, item := range incoming {
		key := todoBoardKey(item)
		if key == "" {
			continue
		}
		if todoBoardFinished(item) {
			finished[key] = struct{}{}
			delete(index, key)
			if _, done := archived[key]; !done {
				archived[key] = struct{}{}
				archive = append(archive, item)
			}
			continue
		}
		if at, ok := index[key]; ok {
			queue[at] = item
			continue
		}
		index[key] = len(queue)
		queue = append(queue, item)
	}

	// Anything this list called finished leaves the queue, including the stale
	// copy of an item that was queued under an older status.
	trimmed := queue[:0]
	for _, item := range queue {
		if _, done := finished[todoBoardKey(item)]; done {
			continue
		}
		trimmed = append(trimmed, item)
	}
	queue = trimmed

	return TodoBoard{Queue: tailOf(queue, todoBoardLimit), Archive: tailOf(archive, todoBoardLimit)}
}

// todoBoardKey identifies one item across lists: its text and nesting.
func todoBoardKey(item evidence.TodoItem) string {
	text := strings.TrimSpace(item.Content)
	if text == "" {
		return ""
	}
	return text + "\x00" + strconv.Itoa(item.Level)
}

func todoBoardFinished(item evidence.TodoItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Status)) {
	case "completed", "done", "dropped", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func tailOf(items []evidence.TodoItem, limit int) []evidence.TodoItem {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[len(items)-limit:]
}
