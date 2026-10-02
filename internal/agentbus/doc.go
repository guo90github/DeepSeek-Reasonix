// Package agentbus is the machine channel of the AGENT_BUS contract: the view a
// participant may see, with the envelope and the talk tiers following.
//
// Everything here is a pure projection of the folded blackboard
// (internal/agentbus/board): no log reads, no clock, no network. That is what
// lets the boundary tests assert exactly what reaches a provider request — see
// docs/agents/AGENT_BUS.md §13 for the size, ordering and truncation rules.
package agentbus
