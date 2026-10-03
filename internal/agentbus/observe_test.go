package agentbus

import (
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func obsNode(id string, state board.NodeState) *board.Node {
	return &board.Node{ID: id, State: state}
}

func obsState(nodes ...*board.Node) *board.State {
	out := &board.State{Nodes: map[string]*board.Node{}}
	for _, n := range nodes {
		out.Nodes[n.ID] = n
	}
	return out
}

func signalNodes(digest *Briefing, kind SignalKind) []string {
	out := []string{}
	for _, signal := range digest.Signals {
		if signal.Kind == kind {
			out = append(out, signal.Node)
		}
	}
	return out
}

// Work addressed to one participant is not waiting for a slot: if the assignee never
// comes, nobody else may take it, so the first screen has to name who and how long.
func TestObserveNamesWorkAddressedToSomeoneWhoNeverCame(t *testing.T) {
	step := obsNode("step", board.StateOpen)
	step.Assignee = "bob"

	long := NewQueueState()
	if _, err := ApplyQueue(long, QueueRecord{Kind: QueueEnqueue, Node: "step", Subtree: "step", Participant: "bob", At: talkBase.Add(-time.Hour)}, QueueLimits{}); err != nil {
		t.Fatalf("park: %v", err)
	}
	digest := Observe(obsState(step), long, nil, talkBase, ObserveLimits{})
	reported := false
	for _, signal := range digest.Signals {
		if signal.Kind != SignalStalled || signal.Node != "step" {
			continue
		}
		reported = true
		if !strings.Contains(signal.Detail, "bob") {
			t.Fatalf("detail = %q, want the assignee named", signal.Detail)
		}
	}
	if !reported {
		t.Fatalf("signals = %+v, want the assigned step reported as stalled", digest.Signals)
	}

	fresh := NewQueueState()
	if _, err := ApplyQueue(fresh, QueueRecord{Kind: QueueEnqueue, Node: "step", Subtree: "step", Participant: "bob", At: talkBase}, QueueLimits{}); err != nil {
		t.Fatalf("park: %v", err)
	}
	if digest := Observe(obsState(step), fresh, nil, talkBase, ObserveLimits{}); len(digest.Signals) != 0 {
		t.Fatalf("signals = %+v, want the assignee's window respected", digest.Signals)
	}
}

func TestObserveShowsOnlySubtreesThatNeedAttention(t *testing.T) {
	healthy := obsNode("ok1", board.StateDone)
	healthy.Owner = "alice"
	busy := obsNode("ok2", board.StateClaimed)
	busy.Owner = "bob"
	busy.Deadline = talkBase.Add(time.Hour)

	digest := Observe(obsState(healthy, busy), nil, nil, talkBase, ObserveLimits{})
	if len(digest.Cards) != 0 {
		t.Fatalf("cards = %+v, want none: healthy work is a number, not a card", digest.Cards)
	}
	if digest.HealthySubtrees != 2 {
		t.Fatalf("healthy subtrees = %d, want the two quiet ones", digest.HealthySubtrees)
	}
	if len(digest.Signals) != 0 {
		t.Fatalf("signals = %+v, want none", digest.Signals)
	}
}

func TestObserveAlwaysShowsOrphansAndStalls(t *testing.T) {
	orphan := obsNode("orphan", board.StateOpen)
	orphan.Deps = []string{"gone"}
	stalled := obsNode("stalled", board.StateClaimed)
	stalled.Owner = "bob"
	stalled.Deadline = talkBase.Add(-time.Minute)
	stuck := obsNode("stuck", board.StateOpen)
	stuck.NoProgress = 2
	disputed := obsNode("disputed", board.StateContested)
	disputed.Refutes = []board.Refutation{{Actor: "alice"}}
	contested := obsNode("contested", board.StateContested)
	contested.Refutes = []board.Refutation{{Actor: "bob"}}

	digest := Observe(obsState(orphan, stalled, stuck, disputed, contested), nil, nil, talkBase, ObserveLimits{MaxSignals: 1})
	// MaxSignals bounds the optional signals only: the orphan and both stalls are
	// still there, one dispute fits, and the one that did not is counted, not lost.
	if got := signalNodes(digest, SignalOrphan); len(got) != 1 || got[0] != "orphan" {
		t.Fatalf("orphans = %v, want the orphan kept", got)
	}
	if got := signalNodes(digest, SignalStalled); len(got) != 2 {
		t.Fatalf("stalls = %v, want both stalls kept whatever the cap", got)
	}
	if got := signalNodes(digest, SignalDisputed); len(got) != 1 {
		t.Fatalf("disputes = %v, want room for one optional signal", got)
	}
	if digest.Hidden != 1 {
		t.Fatalf("hidden = %d, want the dropped signal counted", digest.Hidden)
	}
	if len(digest.Signals) == 0 || !digest.Signals[0].Kind.Mandatory() {
		t.Fatalf("signals = %+v, want the mandatory ones first", digest.Signals)
	}
}

func TestObserveNamesTheReasonForEachSignal(t *testing.T) {
	cases := []struct {
		name string
		node *board.Node
		want SignalKind
	}{
		{"a missing dependency", &board.Node{ID: "n", State: board.StateOpen, Deps: []string{"gone"}}, SignalOrphan},
		{"a lapsed lease", &board.Node{ID: "n", State: board.StateClaimed,
			Owner: "bob", Deadline: talkBase.Add(-time.Second)}, SignalStalled},
		{"recorded no progress", &board.Node{ID: "n", State: board.StateOpen, NoProgress: 3}, SignalStalled},
		{"refuted and not yet ruled on", &board.Node{ID: "n", State: board.StateContested,
			Refutes: []board.Refutation{{Actor: "alice"}}}, SignalDisputed},
		{"refuted after it was decided", &board.Node{ID: "n", State: board.StateContested,
			Outcome: board.OutcomeDone, Refutes: []board.Refutation{{Actor: "alice"}}}, SignalDisputed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			digest := Observe(obsState(tc.node), nil, nil, talkBase, ObserveLimits{})
			if len(digest.Signals) != 1 || digest.Signals[0].Kind != tc.want {
				t.Fatalf("signals = %+v, want one %s", digest.Signals, tc.want)
			}
			if digest.Signals[0].Detail == "" {
				t.Fatal("a signal must say why it is one")
			}
			if digest.Cards[0].Worst != tc.want {
				t.Fatalf("card worst = %q, want %q", digest.Cards[0].Worst, tc.want)
			}
		})
	}
}

// T8-4: a dispute that arrived *after* the verdict still has to be visible. A refutation
// puts a finished step back into `contested`, so the state machine knows it even though the
// node's Outcome still says done — and a hand-made state cannot show that, so this one is
// folded from real ops.
func TestObserveReportsADisputeThatArrivedAfterTheVerdict(t *testing.T) {
	state := testState(t,
		assertOp("publish", "planner"),
		claimOp("publish", "worker"),
		doneOp("publish", "worker", "verifier"),
		board.Op{
			Verb: board.VerbRefute, Node: "publish", Actor: "skeptic",
			Reason: "the test does not cover the migration",
		},
	)
	node := state.Nodes["publish"]
	if node.State != board.StateContested || node.Outcome != board.OutcomeDone {
		t.Fatalf("setup: publish = %s/%s, want contested with its old verdict still recorded",
			node.State, node.Outcome)
	}
	digest := Observe(state, nil, nil, talkBase, ObserveLimits{})
	if got := signalNodes(digest, SignalDisputed); len(got) != 1 || got[0] != "publish" {
		t.Fatalf("disputes = %v, want the step that was refuted after it finished", got)
	}
}

func TestObserveReportsDeliberations(t *testing.T) {
	hearings := NewHearingState()
	if err := ApplyHearing(hearings, openHearing("n1", []string{"alice"}, talkBase), HearingLimits{}); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := ApplyHearing(hearings, ruleHearing("n1", VerdictEscalate, ReasonEqualWeight, "orchestrator", talkBase.Add(time.Second)), HearingLimits{}); err != nil {
		t.Fatalf("escalate: %v", err)
	}
	if err := ApplyHearing(hearings, openHearing("n2", []string{"alice"}, talkBase.Add(2*time.Second)), HearingLimits{}); err != nil {
		t.Fatalf("open n2: %v", err)
	}
	if err := ApplyHearing(hearings, ruleHearing("n2", VerdictUndecided, ReasonQuota, "orchestrator", talkBase.Add(3*time.Second)), HearingLimits{}); err != nil {
		t.Fatalf("rule n2: %v", err)
	}
	if err := ApplyHearing(hearings, openHearing("n3", []string{"alice"}, talkBase.Add(4*time.Second)), HearingLimits{}); err != nil {
		t.Fatalf("open n3: %v", err)
	}

	digest := Observe(nil, nil, hearings, talkBase, ObserveLimits{})
	if got := signalNodes(digest, SignalEscalated); len(got) != 1 || got[0] != "n1" {
		t.Fatalf("escalated = %v, want n1", got)
	}
	if got := signalNodes(digest, SignalUndecided); len(got) != 1 || got[0] != "n2" {
		t.Fatalf("undecided = %v, want n2 on the record", got)
	}
	if got := signalNodes(digest, SignalDisputed); len(got) != 1 || got[0] != "n3" {
		t.Fatalf("disputed = %v, want the open deliberation", got)
	}
}

func TestObserveFoldsBySubtreeAndCountsWhatItHides(t *testing.T) {
	root := obsNode("root", board.StateOpen)
	root.Deps = []string{"gone"}
	mid := obsNode("mid", board.StateClaimed)
	mid.Owner = "bob"
	mid.Deps = []string{"root"}
	mid.Deadline = talkBase.Add(-time.Minute)
	leaf := obsNode("leaf", board.StateDone)
	leaf.Deps = []string{"mid"}
	other := obsNode("other", board.StateClaimed)
	other.Owner = "carol"
	other.Deadline = talkBase.Add(-time.Minute)

	queue := NewQueueState()
	if _, err := ApplyQueue(queue, QueueRecord{Kind: QueueEnqueue, Node: "parked", Subtree: "root", At: talkBase}, QueueLimits{}); err != nil {
		t.Fatalf("park: %v", err)
	}

	digest := Observe(obsState(root, mid, leaf, other), queue, nil, talkBase, ObserveLimits{MaxCards: 1})
	if len(digest.Cards) != 1 {
		t.Fatalf("cards = %+v, want the cap respected", digest.Cards)
	}
	if digest.HiddenCards != 1 {
		t.Fatalf("hidden cards = %d, want the second attention subtree counted", digest.HiddenCards)
	}
	card := digest.Cards[0]
	if card.Subtree != "root" || card.Nodes != 3 || card.AtWork != 1 || card.Done != 1 || card.Parked != 1 {
		t.Fatalf("card = %+v, want the root subtree folded with its counts", card)
	}
	if card.Stalled != 1 || card.Orphans != 1 || card.Signals != 2 || card.Worst != SignalOrphan {
		t.Fatalf("card = %+v, want both wrongs reported with the orphan first", card)
	}
}
