package board

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Verb is the closed set of blackboard operations.
type Verb string

const (
	VerbAssert        Verb = "assert"
	VerbRefute        Verb = "refute"
	VerbClaim         Verb = "claim"
	VerbHeartbeat     Verb = "heartbeat"
	VerbRelease       Verb = "release"
	VerbYield         Verb = "yield"
	VerbSplit         Verb = "split"
	VerbRequire       Verb = "require"
	VerbAssign        Verb = "assign"
	VerbDecide        Verb = "decide"
	VerbAbandon       Verb = "abandon"
	VerbRevert        Verb = "revert"
	VerbCapabilityGap Verb = "capability_gap"
	VerbNoProgress    Verb = "no_progress"
)

// ActorSystem owns system records such as the reclamation no_progress op.
const ActorSystem = "system"

// Outcome is the verdict a decide op carries.
type Outcome string

const (
	OutcomeDone      Outcome = "done"
	OutcomeBlocked   Outcome = "blocked"
	OutcomeAbandoned Outcome = "abandoned"
)

// Evidence is one re-checkable pointer backing an assert or an abandon request.
type Evidence struct {
	Kind string `json:"kind,omitempty"`
	Ref  string `json:"ref"`
	Note string `json:"note,omitempty"`
}

// NodeSpec names a node created by split or require.
type NodeSpec struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// Bounds is a claim's declaration of what the work is allowed to cost.
type Bounds struct {
	Steps  int    `json:"steps,omitempty"`
	Tokens int64  `json:"tokens,omitempty"`
	Output string `json:"output,omitempty"`
}

// Op is one blackboard operation as the log stores it.
type Op struct {
	ID           string     `json:"id"`
	Seq          uint64     `json:"seq"`
	Verb         Verb       `json:"verb"`
	Node         string     `json:"node,omitempty"`
	Actor        string     `json:"actor,omitempty"`
	At           time.Time  `json:"at"`
	Deadline     time.Time  `json:"deadline,omitzero"`
	Outcome      Outcome    `json:"outcome,omitempty"`
	Evidence     []Evidence `json:"evidence,omitempty"`
	ReproducedBy string     `json:"reproducedBy,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	// Title names a node an operation creates: require and split name their children, and
	// assert names the node it lays down (2026-10-03).
	Title    string     `json:"title,omitempty"`
	Children []NodeSpec `json:"children,omitempty"`
	Dep      *NodeSpec  `json:"dep,omitempty"`
	Assignee string     `json:"assignee,omitempty"`
	Bounds   *Bounds    `json:"bounds,omitempty"`
	Source   string     `json:"source,omitempty"`
}

// DeriveID builds the idempotency key for an op that did not carry one. It
// covers the verb, node, actor and payload, so a retried delivery of the same
// intent collapses onto one op while a different intent never does. Time is
// deliberately excluded: a retry hours later is still the same intent.
func DeriveID(op Op) string {
	payload := struct {
		Verb         Verb       `json:"verb"`
		Node         string     `json:"node"`
		Actor        string     `json:"actor"`
		Outcome      Outcome    `json:"outcome"`
		Evidence     []Evidence `json:"evidence"`
		ReproducedBy string     `json:"reproducedBy"`
		Reason       string     `json:"reason"`
		Title        string     `json:"title,omitempty"`
		Children     []NodeSpec `json:"children"`
		Dep          *NodeSpec  `json:"dep"`
		Assignee     string     `json:"assignee"`
		Bounds       *Bounds    `json:"bounds"`
	}{
		Verb: op.Verb, Node: op.Node, Actor: op.Actor, Outcome: op.Outcome,
		Evidence: op.Evidence, ReproducedBy: op.ReproducedBy, Reason: op.Reason,
		Title:    op.Title,
		Children: op.Children, Dep: op.Dep, Bounds: op.Bounds, Assignee: op.Assignee,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "op-" + strings.TrimSpace(string(op.Verb)) + ":" + op.Node
	}
	sum := sha256.Sum256(raw)
	return "op-" + hex.EncodeToString(sum[:])[:20]
}

// SweepID is the idempotency key for reclaiming one expired claim. The deadline
// is part of the key so a later claim by someone else earns its own record.
func SweepID(node string, deadline time.Time) string {
	sum := sha256.Sum256([]byte("sweep|" + node + "|" + deadline.UTC().Format(time.RFC3339Nano)))
	return "sweep-" + hex.EncodeToString(sum[:])[:20]
}

// RejectError explains why an op was refused. Every refusal is typed: a
// silently ignored op is the failure mode this type exists to prevent.
type RejectError struct {
	Verb   Verb
	Node   string
	Reason string
}

func (e *RejectError) Error() string {
	node := e.Node
	if node == "" {
		node = "-"
	}
	return fmt.Sprintf("board: %s rejected on %s: %s", e.Verb, node, e.Reason)
}

// Rejection reasons. Callers and tests match on these strings.
const (
	ReasonUnknownVerb            = "unknown_verb"
	ReasonUnknownNode            = "unknown_node"
	ReasonIllegalTransition      = "illegal_transition"
	ReasonMissingActor           = "missing_actor"
	ReasonMissingNode            = "missing_node"
	ReasonMissingEvidence        = "missing_evidence"
	ReasonMissingReason          = "missing_reason"
	ReasonMissingDeadline        = "missing_deadline"
	ReasonMissingBounds          = "missing_bounds"
	ReasonNotOwner               = "not_owner"
	ReasonNotAssignee            = "not_assignee"
	ReasonMissingReproducer      = "missing_reproducer"
	ReasonSelfReproduced         = "self_reproduced"
	ReasonMissingAbandonRequest  = "missing_abandon_request"
	ReasonMissingDependency      = "missing_dependency"
	ReasonDuplicateDependency    = "duplicate_dependency"
	ReasonDuplicateNode          = "duplicate_node"
	ReasonCycle                  = "cycle"
	ReasonUnknownOutcome         = "unknown_outcome"
	ReasonInvalidOutcomeForState = "invalid_outcome_for_state"
	ReasonDeadlineNotFuture      = "deadline_not_in_future"
	ReasonSystemOnly             = "system_only"
	ReasonDependencyClosed       = "dependency_closed"
	ReasonIdempotencyConflict    = "idempotency_conflict"
	// ReasonRateLimited is the tier AGENT_BUS §S5 names for "not now, come back":
	// the node is moving faster than the operator allows.
	ReasonRateLimited = "rate_limited"
)

func reject(verb Verb, node, reason string) error {
	return &RejectError{Verb: verb, Node: node, Reason: reason}
}

// IsReject reports whether err is a typed refusal and returns its reason.
func IsReject(err error) (string, bool) {
	var rej *RejectError
	if errors.As(err, &rej) {
		return rej.Reason, true
	}
	return "", false
}

func hasEvidence(ev []Evidence) bool {
	for _, e := range ev {
		if strings.TrimSpace(e.Ref) != "" {
			return true
		}
	}
	return false
}
