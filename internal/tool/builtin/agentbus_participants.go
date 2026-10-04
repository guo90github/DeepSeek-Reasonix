package builtin

import (
	"fmt"
	"strings"
	"time"
)

// readParticipants renders who is on this board right now. The roster is the board's own
// address book, and until the desktop announced itself it held only departures — so a session
// could not tell who it was working beside (F48/F49, 2026-10-05).
func (t agentBusBoard) readParticipants() (string, error) {
	participant, boardDir, err := t.identity()
	if err != nil {
		return "", err
	}
	refs, err := t.port.BoardParticipants()
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("board %s as %s\n", boardName(boardDir), participant)
	if len(refs) == 0 {
		return head + "nobody is on this board: a session appears here when it joins and leaves when it withdraws", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d on this board now:\n", len(refs))
	for _, ref := range refs {
		who := ref.Participant
		if who == participant {
			who += " (you)"
		}
		fmt.Fprintf(&b, "- %s", who)
		if ref.SessionPath != "" {
			fmt.Fprintf(&b, " session=%s", ref.SessionPath)
		}
		if ref.Host != "" {
			fmt.Fprintf(&b, " host=%s", ref.Host)
		}
		if !ref.At.IsZero() {
			fmt.Fprintf(&b, " announced=%s", ref.At.UTC().Format(time.RFC3339))
		}
		b.WriteString("\n")
	}
	return head + strings.TrimRight(b.String(), "\n"), nil
}
