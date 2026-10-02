package agentbus

import (
	"strings"

	"reasonix/internal/agentbus/board"
)

// GrantSource marks the assertion that authorizes a node to obtain what it lacks.
// A grant rides `assert` instead of a new verb: S1's verb set is a frozen closed set
// with its own migration table and refusals, and "this work may touch that" is an
// assertion about the node (AGENT_BUS §13.8).
const GrantSource = "agentbus-grant"

// Grant is one authorization as the op log records it.
type Grant struct {
	Node     string
	Actor    string
	Reason   string
	Evidence []board.Evidence
	Seq      uint64
}

// GrantsFor reads the authorizations a node has been given. The folded state cannot
// answer this — `Assertion` carries no `Source` — so provenance comes from the ops,
// the same precedent as reading ops for a wake target's requester (§13.3, §13.8).
func GrantsFor(ops []board.Op, node string) []Grant {
	out := []Grant{}
	node = strings.TrimSpace(node)
	if node == "" {
		return out
	}
	for _, op := range ops {
		if op.Verb != board.VerbAssert || op.Node != node || op.Source != GrantSource {
			continue
		}
		out = append(out, Grant{
			Node: op.Node, Actor: op.Actor, Reason: op.Reason, Evidence: op.Evidence, Seq: op.Seq,
		})
	}
	return out
}

// Authorized reports whether a node holds an authorization it may act on. Three rules
// keep the answer honest: the grantor must not be one of the node's own producers
// (nobody authorizes themselves), the grant must carry evidence someone can check, and a
// node that was abandoned or taken back holds nothing — its authorization went with it.
func Authorized(ops []board.Op, state *board.State, node string) bool {
	return len(AuthorizedGrants(ops, state, node)) > 0
}

// producersOf reads who has been this node's producer: anybody who asserted it as work or
// held its lease. The folded owner names only the *current* lease holder, and a capability
// gap clears it — exactly when a fresh authorization is needed — so whose work this is has
// to come from the log. Holding the lease counts on its own: the worker that reports a gap
// may never have asserted anything.
func producersOf(ops []board.Op, node string) map[string]bool {
	out := map[string]bool{}
	for _, op := range ops {
		if op.Node != node || strings.TrimSpace(op.Actor) == "" {
			continue
		}
		switch op.Verb {
		case board.VerbAssert:
			if op.Source != GrantSource {
				out[op.Actor] = true
			}
		case board.VerbClaim:
			out[op.Actor] = true
		}
	}
	return out
}

// AuthorizedGrants returns the grants that actually authorize a node, so a caller can
// show who approved what rather than only whether anybody did.
func AuthorizedGrants(ops []board.Op, state *board.State, node string) []Grant {
	if state != nil {
		if n, ok := state.Nodes[node]; ok {
			if n.State == board.StateAbandoned || n.State == board.StateStale {
				return nil
			}
		}
	}
	producers := producersOf(ops, node)
	out := []Grant{}
	for _, grant := range GrantsFor(ops, node) {
		if grant.Actor == "" || producers[grant.Actor] || len(grant.Evidence) == 0 {
			continue
		}
		out = append(out, grant)
	}
	return out
}
