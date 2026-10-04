package control

import (
	"fmt"
	"strings"

	"reasonix/internal/agentbus"
)

// agentBusWakeMaxBytes bounds one wake block: a wake is a pointer to work, not a digest of the
// board, and a board with hundreds of nodes would otherwise put the whole board in the prompt
// (measured 2026-10-04: 11 wake blocks, 4.8 KB, on one busy session).
const agentBusWakeMaxBytes = 2048

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
	b.WriteString(wakeGuidanceLine(target))
	writeWakeList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(label)
		b.WriteString(": ")
		shown := 0
		for i, item := range items {
			if b.Len()+len(item)+2 > agentBusWakeMaxBytes {
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(item)
			shown++
		}
		// A cut list says what it cut and where the rest is: a silently shortened list reads
		// like the whole answer, and the reader then believes work does not exist.
		if shown < len(items) {
			fmt.Fprintf(&b, " …and %d more (action=view)", len(items)-shown)
		}
		b.WriteString("\n")
	}
	writeWakeList("startable now", target.Ready)
	writeWakeList("addressed to you, and nobody else may take it", target.Assigned)
	writeWakeList("waiting on you", target.Waiting)
	writeWakeList("questions addressed to you", target.Asks)
	writeWakeList("deliberations you owe an answer about", target.Owes)
	// The host already stopped handing these out, so the line has to say why: without the
	// reason it reads like any other "startable" line (G5).
	writeWakeList(fmt.Sprintf("steps that stopped moving (handed out %d times with no progress): take one, replan it, or say why it cannot move", agentBusDispatchTries), target.Stalled)
	b.WriteString("</agentbus-wake>\n")
	return b.String()
}

// wakeGuidanceLine names the surface the lists above actually live on: a question and a
// deliberation are talk-side and never become board rows, so telling their addressee to read the
// board sends it looking for a row that is not there (measured 2026-10-04).
func wakeGuidanceLine(target agentbus.WakeTarget) string {
	const boardGuidance = "Sent when the board last changed; read the board before acting on this list.\n"
	if len(target.Ready)+len(target.Assigned)+len(target.Waiting)+len(target.Stalled) > 0 {
		return boardGuidance
	}
	asks, owes := len(target.Asks) > 0, len(target.Owes) > 0
	switch {
	case asks && owes:
		return "Sent when the board last changed; neither list above is a board row — answer the question with the agent_bus tool (action=answer, correlation=…), the deliberation you owe with action=hearing_answer, node=….\n"
	case asks:
		return "Sent when the board last changed; what is listed above is a question, not a board row — answer it with the agent_bus tool (action=answer, correlation=…).\n"
	case owes:
		return "Sent when the board last changed; what is listed above is a deliberation, not a board row — answer it with the agent_bus tool (action=hearing_answer, node=…).\n"
	default:
		return boardGuidance
	}
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
	appendList(fmt.Sprintf("stopped moving (handed out %d times)", agentBusDispatchTries), target.Stalled)
	return b.String()
}
