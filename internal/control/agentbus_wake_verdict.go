package control

import (
	"fmt"

	"reasonix/internal/agentbus/board"
)

// assertionsWithEvidence counts the readings a node already carries. A count is not a
// quality judgement — the board's own row counts assertions the same way — it only says
// somebody has reported here (2026-10-05).
func assertionsWithEvidence(n *board.Node) int {
	if n == nil {
		return 0
	}
	count := 0
	for _, a := range n.Asserts {
		if len(a.Evidence) > 0 {
			count++
		}
	}
	return count
}

// agentBusDispatchVerdictLine is the sentence a step that already carries readings gets
// instead of "do it, then decide it": that instruction is what sent real holders back to
// write the same reading again (measured 2026-10-05: one probe reached 21 assertions with
// nothing proved).
func agentBusDispatchVerdictLine(node string, assertions int) string {
	return fmt.Sprintf("It already carries %d assertion(s) with evidence: read the node (%s, action=view) before spending the turn — if those readings hold, this step needs a verdict (decide), not another run of the same work.", assertions, node)
}

// agentBusDispatchVerdictBlock is that line as a whole injected block.
func agentBusDispatchVerdictBlock(node string, assertions int) string {
	return "<agentbus-wake>\nThe board assigned this step to you: " + node + "\n" + agentBusDispatchVerdictLine(node, assertions) + "\n</agentbus-wake>\n"
}
