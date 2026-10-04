package builtin

import (
	"fmt"
	"strings"
)

// checkAssigneeOnBoard refuses an assignment to a name the roster does not know. The step would
// otherwise be reachable by nobody: the board refuses a stranger's claim, the wake skips a step
// that carries an assignee, and the queue keeps an entry no participant will ever take
// (F39/F50, 2026-10-05). A roster that is empty or unreadable is no evidence, so it refuses
// nothing: a board nobody has announced on yet must still take an assignment.
func (t agentBusBoard) checkAssigneeOnBoard(assignee string) error {
	name := strings.TrimSpace(assignee)
	refs, err := t.port.BoardParticipants()
	if err != nil {
		return nil
	}
	known := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Participant == "" {
			continue
		}
		if ref.Participant == name {
			return nil
		}
		known = append(known, ref.Participant)
	}
	if len(known) == 0 {
		return nil
	}
	return fmt.Errorf("assign needs a participant who is on the board: nobody named %q is; on this board now: %s — use action=participants to see them in full", name, strings.Join(known, ", "))
}
