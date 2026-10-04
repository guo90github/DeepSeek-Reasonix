package control

import "strings"

// SetAgentBusRole declares what this host calls the sessions it runs, so a reader of the roster
// can see what a participant is for. The value is the operator's — the kernel has no role
// vocabulary of its own — and an empty one simply leaves the column off. A host re-declares it
// at enrolment, the same path the hearing and talk bounds take (F48, 2026-10-05).
func (c *Controller) SetAgentBusRole(role string) {
	c.mu.Lock()
	state := c.agentBus
	c.mu.Unlock()
	if state == nil {
		return
	}
	state.mu.Lock()
	state.role = strings.TrimSpace(role)
	state.mu.Unlock()
}
