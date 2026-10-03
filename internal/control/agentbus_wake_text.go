package control

import (
	"fmt"
	"strings"

	"reasonix/internal/agentbus"
)

// AgentBusWakePrompt renders the block a woken session reads.
//
// It lives here, not in a frontend, because the wake is *rendered* where it is enqueued and
// *injected* when the turn ends — minutes later on a long turn — while the board may already
// have moved on. Both sites have to render the same fact from the same target, so the injection
// site can rebuild it instead of injecting a frozen list. Today the host still calls through
// (desktop/agentbus_waker.go); the stamp in the text keeps the frozen case honest in the
// meantime (found on a real machine: a wake arrived 4.5 minutes after its list had stopped being
// true, 2026-10-03).
func AgentBusWakePrompt(target agentbus.WakeTarget) string {
	if agentbus.IsDispatchKey(target.Key) {
		return fmt.Sprintf("<agentbus-wake>\nThe board assigned this work to you: %s\nIt is already claimed in your name: do it, then decide it.\nSent when the board assigned it: if that lease has lapsed since, re-read the board before acting.\nWorking on it for longer than the lease? Renew it with the agentbus heartbeat, or the board takes the claim back.\n</agentbus-wake>\n",
			strings.Join(target.Ready, ", "))
	}
	var b strings.Builder
	b.WriteString("<agentbus-wake>\n")
	b.WriteString("The board woke you: it has work only you can move right now.\n")
	b.WriteString("Sent when the board last changed; read the board before acting on this list.\n")
	writeWakeList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.Join(items, ", "))
		b.WriteString("\n")
	}
	writeWakeList("startable now", target.Ready)
	writeWakeList("waiting on you", target.Waiting)
	writeWakeList("questions addressed to you", target.Asks)
	writeWakeList("deliberations you owe an answer about", target.Owes)
	b.WriteString("</agentbus-wake>\n")
	return b.String()
}
