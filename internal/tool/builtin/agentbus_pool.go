package builtin

import (
	"fmt"
	"strings"
)

// readPool renders the board's common pool: steps that can start, that nobody holds, and that
// nobody is waiting on. Nothing broadcasts them — the wake names requesters, and the dispatcher
// takes only work with a waiter — so asking is the one way a session can offer to take one
// (F53/F40, 2026-10-05).
func (t agentBusBoard) readPool() (string, error) {
	participant, boardDir, err := t.identity()
	if err != nil {
		return "", err
	}
	entries, err := t.port.BoardPool()
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("board %s as %s\n", boardName(boardDir), participant)
	if len(entries) == 0 {
		return head + "the pool is empty: every step that can start is held, addressed to somebody, or nobody moved it yet", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d step(s) in the pool (claim one to take it):\n", len(entries))
	for _, entry := range entries {
		fmt.Fprintf(&b, "- %s", entry.ID)
		if entry.Title != "" {
			fmt.Fprintf(&b, " title=%q", entry.Title)
		}
		fmt.Fprintf(&b, " waiters=%d last_seq=%d\n", entry.Waiters, entry.LastSeq)
	}
	return head + strings.TrimRight(b.String(), "\n"), nil
}
