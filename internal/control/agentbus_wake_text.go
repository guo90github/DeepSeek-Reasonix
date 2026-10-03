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
	writeWakeList("addressed to you, and nobody else may take it", target.Assigned)
	writeWakeList("waiting on you", target.Waiting)
	writeWakeList("questions addressed to you", target.Asks)
	writeWakeList("deliberations you owe an answer about", target.Owes)
	b.WriteString("</agentbus-wake>\n")
	return b.String()
}

// AgentBusWakeSource is the source a wake's inbox item carries. It names the producer, so a
// host that shows what arrived can say who woke the session.
const AgentBusWakeSource = "agentbus"

// AgentBusWakeLine says the same thing to a person: the block above is what the model reads,
// and the queue that shows this to whoever is looking must not render XML at them.
//
// It sits beside the prompt because both hosts need it: the desktop renders it for its own
// delivery, and a headless host with no renderer of its own (serve) would otherwise have to
// put the XML in front of a person.
func AgentBusWakeLine(target agentbus.WakeTarget) string {
	if agentbus.IsDispatchKey(target.Key) {
		return "已指派给你：" + strings.Join(target.Ready, ", ")
	}
	var b strings.Builder
	b.WriteString("The board has work for you")
	appendList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("; ")
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.Join(items, ", "))
	}
	appendList("startable now", target.Ready)
	appendList("addressed to you", target.Assigned)
	appendList("waiting on you", target.Waiting)
	appendList("questions for you", target.Asks)
	appendList("deliberations you owe", target.Owes)
	return b.String()
}
