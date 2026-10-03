package main

import (
	"context"
	"net/http"

	"reasonix/internal/agentbus"
)

// agentBusSessionHeader carries the addressed session. It is wire protocol between hosts, so
// it is stated once in the kernel and aliased here rather than guessed at each call site.
const agentBusSessionHeader = agentbus.SessionPathHeader

// AgentBusDelivery is where one wake goes when the participant is not in this process: the
// host's base URL, the bearer token that host published, and the session header it reads to
// know which session the wake is for.
type AgentBusDelivery = agentbus.WakeDelivery

// deliverAgentBusWake posts a wake to another host. The wire shape lives in the kernel
// (agentbus.DeliverWake) because a headless host delivers the same wake, and two copies of a
// protocol is how the desktop and serve drift apart on it.
func deliverAgentBusWake(ctx context.Context, client *http.Client, delivery AgentBusDelivery, target agentbus.WakeTarget) error {
	return agentbus.DeliverWake(ctx, client, delivery, agentbus.WakeMessage{
		Prompt:  agentBusWakePrompt(target),
		Display: agentBusWakeLine(target),
		Key:     target.Key,
	})
}
