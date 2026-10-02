package agentbus

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

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
	// Waiting are nodes of theirs that cannot start until Ready is done.
	Waiting []string
	// Asks are questions addressed to this participant that nobody has answered.
	Asks []string
}

// WakeTargets lists who should be woken now.
//
//   - whoever required a node that is startable and still unowned hears that the
//     step can run;
//   - whoever a question was addressed to hears that an answer is owed.
//
// It reads the op log rather than the folded board on purpose: who *asked* for an
// unowned node is not in the fold, only in the op that asked. Teaching nodes to
// carry their requester would move this back into the fold.
func WakeTargets(ops []board.Op, talk *TalkState) []WakeTarget {
	targets := map[string]*WakeTarget{}
	target := func(participant string) *WakeTarget {
		if t := targets[participant]; t != nil {
			return t
		}
		t := &WakeTarget{Participant: participant}
		targets[participant] = t
		return t
	}

	st := board.Fold(ops)
	for _, op := range ops {
		if op.Verb != board.VerbRequire || op.Dep == nil || op.Actor == "" {
			continue
		}
		dep, ok := st.Nodes[op.Dep.ID]
		if !ok || !dep.Ready(st) {
			continue
		}
		if requester, ok := st.Nodes[op.Node]; ok && requester.State == board.StateDone {
			continue
		}
		t := target(op.Actor)
		t.Ready = appendUnique(t.Ready, op.Dep.ID)
		if op.Node != "" {
			t.Waiting = appendUnique(t.Waiting, op.Node)
		}
	}

	if talk != nil {
		for _, id := range sortedChainIDs(talk) {
			chain := talk.Chains[id]
			// One hop means the question was asked and never answered: an answer
			// would have spent a second hop.
			if chain.Hops != 1 {
				continue
			}
			addressee := askTarget(talk, id)
			if addressee == "" {
				continue
			}
			t := target(addressee)
			t.Asks = appendUnique(t.Asks, id)
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
	for _, group := range [][]string{t.Ready, t.Waiting, t.Asks} {
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
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func sortedChainIDs(talk *TalkState) []string {
	out := make([]string, 0, len(talk.Chains))
	for id := range talk.Chains {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
