package event

// DeliveryClass says how an item reaches the model. The secondary line — anything
// the session did not itself produce — must declare one of these at its
// construction point (docs/50 §2.3):
//
//   - ExternalGuidance (A) rides the turn body, marked, and never the system
//     prefix. Exactly two entry points: the inbox wake path and the hook context
//     block (pinned by internal/control/turn_tail_sites_test.go).
//   - HostNotice (B) rides the event channel: notices about the run itself
//     (truncation, compaction, a blocked call) never enter the turn body.
//   - Projection (C) is a read-only view of state (panels, dock, task shelf).
//   - SessionState is the main line's own state (goal marker, plan marker, memory
//     update, recall block, job note), listed only so every turn-body site is
//     classified.
type DeliveryClass string

const (
	DeliverySessionState     DeliveryClass = "session-state"
	DeliveryExternalGuidance DeliveryClass = "external-guidance"
	DeliveryHostNotice       DeliveryClass = "host-notice"
	DeliveryProjection       DeliveryClass = "projection"
)
