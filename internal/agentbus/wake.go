package agentbus

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus/board"
)

// WakeTarget is one participant who has work waiting on them. The kernel decides
// who and why; routing the wake is the host's job, because only the host knows
// which session, tab or process owns a participant (AGENT_BUS §13.2).
type WakeTarget struct {
	Participant string
	// Key collapses repeated wakes for the same work. It derives from what is
	// waiting, never from when: re-deriving the same state must not wake twice.
	Key string
	// Ready are unowned nodes this participant asked for and that can start now.
	Ready []string
	// Assigned are nodes the board addressed to this participant: unowned and startable,
	// and takeable by nobody else, so the wake has to say that they are theirs (§13.4).
	Assigned []string
	// Waiting are nodes of theirs that cannot start until Ready is done.
	Waiting []string
	// Asks are questions addressed to this participant that nobody has answered.
	Asks []string
	// Owes are deliberations this participant was required to answer and has not,
	// although the round's window has passed.
	Owes []string
	// Stalled are nodes this participant asked for that have spent the host's retry
	// budget: the dispatcher will not hand them out again, so being told is the only way
	// they move (§13.13).
	Stalled []string
}

// DispatchKeyPrefix marks the wake that hands one assignment to a participant rather
// than asking it to start something: the host wrote the claim itself, so the message
// says what the recipient owns, not what it may pick up.
const DispatchKeyPrefix = "agentbus-dispatch:"

// DispatchKey names the wake that carries one assignment. It derives from the board and
// the node, never from the attempt: re-delivering the same assignment collapses on this
// key, while the claim op that lands it is named per attempt by its deadline.
func DispatchKey(board, node string) string {
	return DispatchKeyPrefix + board + "/" + node
}

// IsDispatchKey reports whether a wake is an assignment the host already claimed in the
// participant's name.
func IsDispatchKey(key string) bool {
	return strings.HasPrefix(key, DispatchKeyPrefix)
}

// WakeInput is everything a wake decision reads: the folded board for who asked for what,
// the talk surface for open questions, and the deliberation surface for who still owes an
// answer.
type WakeInput struct {
	State    *board.State
	Talk     *TalkState
	Hearings *HearingState
	Limits   HearingLimits
	Now      time.Time
	// StallAfter is the host's retry budget for one step: a node whose NoProgress has
	// reached it is not handed out again, and the wake says so. Zero means this host keeps
	// no such budget, so nothing is ever reported as stalled — the kernel invents no limit.
	StallAfter int
}

// waitersByDep maps each dependency to the nodes still waiting on it. A finished node is not
// waiting for anything, so it is not listed.
func waitersByDep(st *board.State) map[string][]string {
	out := map[string][]string{}
	if st == nil {
		return out
	}
	for _, id := range sortedNodeIDs(st) {
		n := st.Nodes[id]
		if n.State == board.StateDone {
			continue
		}
		for _, dep := range n.Deps {
			out[dep] = append(out[dep], id)
		}
	}
	return out
}

// WakeTargets lists who should be woken now.
//
//   - whoever asked for a node that can start, is still unowned and still has somebody
//     waiting on it hears that the step can run;
//   - whoever a question was addressed to hears that an answer is owed.
//
// It reads the folded board: who asked for a node is on the node itself (T5-6), so the op
// trail is no longer needed to answer that.
func WakeTargets(in WakeInput) []WakeTarget {
	targets := map[string]*WakeTarget{}
	target := func(participant string) *WakeTarget {
		if t := targets[participant]; t != nil {
			return t
		}
		t := &WakeTarget{Participant: participant}
		targets[participant] = t
		return t
	}

	waiting := waitersByDep(in.State)
	if in.State != nil {
		for _, depID := range sortedNodeIDs(in.State) {
			dep := in.State.Nodes[depID]
			if !dep.Ready(in.State) || stalledNode(dep, in.StallAfter) {
				continue
			}
			// An assigned step is addressed work: it wakes its assignee whether or not
			// anyone is waiting on it, and it wakes nobody else (board.Node.Assignee).
			if assignee := strings.TrimSpace(dep.Assignee); assignee != "" {
				t := target(assignee)
				t.Assigned = appendUnique(t.Assigned, depID)
				for _, blocked := range waiting[depID] {
					t.Waiting = appendUnique(t.Waiting, blocked)
				}
				continue
			}
			// An unowned step that can run now, with somebody still waiting on it: that is
			// the whole reason to wake anyone. Work nobody waits on wakes nobody.
			if len(waiting[depID]) == 0 {
				continue
			}
			for _, requester := range dep.Requesters {
				if requester == "" {
					continue
				}
				t := target(requester)
				t.Ready = appendUnique(t.Ready, depID)
				for _, blocked := range waiting[depID] {
					t.Waiting = appendUnique(t.Waiting, blocked)
				}
			}
		}
	}

	if in.Talk != nil {
		for _, id := range sortedChainIDs(in.Talk) {
			chain := in.Talk.Chains[id]
			// One hop means the question was asked and never answered: an answer
			// would have spent a second hop.
			if chain.Hops != 1 {
				continue
			}
			addressee := askTarget(in.Talk, id)
			if addressee == "" {
				continue
			}
			t := target(addressee)
			t.Asks = appendUnique(t.Asks, id)
		}
	}

	if in.Hearings != nil {
		for _, node := range sortedHearingNodes(in.Hearings) {
			for _, participant := range HearingSilent(in.Hearings.Hearings[node], in.Now, in.Limits) {
				target(participant).Owes = appendUnique(target(participant).Owes, node)
			}
		}
	}

	// Work that has spent the host's budget stops being handed out, and nothing else on the
	// board changes for it: whoever asked for it hears that, or it sits until a person finds
	// it. Only work nobody holds is reported — a live claim is the reclaim's business.
	if in.StallAfter > 0 && in.State != nil {
		for _, id := range sortedNodeIDs(in.State) {
			n := in.State.Nodes[id]
			if !stalledNode(n, in.StallAfter) || n.Owner != "" {
				continue
			}
			for _, requester := range n.Requesters {
				if requester == "" {
					continue
				}
				t := target(requester)
				t.Stalled = appendUnique(t.Stalled, id)
				for _, blocked := range waiting[id] {
					t.Waiting = appendUnique(t.Waiting, blocked)
				}
			}
		}
	}

	out := make([]WakeTarget, 0, len(targets))
	for _, t := range targets {
		t.Key = wakeKey(t)
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Participant < out[j].Participant })
	return out
}

// stalledNode reports work the host will not hand out again: it has spent the retry budget
// that stops a step from being offered forever. A host with no budget (0) stalls nothing.
func stalledNode(n *board.Node, stallAfter int) bool {
	if n == nil || stallAfter <= 0 || n.NoProgress < stallAfter {
		return false
	}
	// A settled node is not a live symptom, whatever its history (T8-2's rule).
	return n.State != board.StateDone && n.State != board.StateAbandoned
}

// askTarget returns who a chain's question was addressed to.
func askTarget(talk *TalkState, correlation string) string {
	for _, name := range talk.TopicNames() {
		for _, line := range talk.Topics[name].Lines {
			if line.Correlation != correlation || line.Kind != TalkAsk {
				continue
			}
			if line.To != "" {
				return line.To
			}
			if len(line.Mentions) == 1 {
				return line.Mentions[0]
			}
		}
	}
	return ""
}

func wakeKey(t *WakeTarget) string {
	sum := sha256.New()
	sum.Write([]byte(t.Participant))
	for _, group := range [][]string{t.Ready, t.Assigned, t.Waiting, t.Asks, t.Owes, t.Stalled} {
		items := append([]string(nil), group...)
		sort.Strings(items)
		for _, item := range items {
			sum.Write([]byte{0})
			sum.Write([]byte(item))
		}
		sum.Write([]byte{1})
	}
	return "agentbus-wake:" + t.Participant + ":" + hex.EncodeToString(sum.Sum(nil)[:8])
}

func appendUnique(items []string, item string) []string {
	if slices.Contains(items, item) {
		return items
	}
	return append(items, item)
}

func sortedHearingNodes(st *HearingState) []string {
	out := make([]string, 0, len(st.Hearings))
	for node := range st.Hearings {
		out = append(out, node)
	}
	sort.Strings(out)
	return out
}

func sortedChainIDs(talk *TalkState) []string {
	out := make([]string, 0, len(talk.Chains))
	for id := range talk.Chains {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
