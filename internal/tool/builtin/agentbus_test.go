package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

type fakeBoardPort struct {
	dir         string
	participant string
	view        agentbus.View
	applied     []board.Op
	reject      error
	duplicate   bool
	identityErr error
	asked       []string
	answered    []string
	hearing     []string
	hearingRec  agentbus.HearingRecord
	hearingErr  error
}

func (f *fakeBoardPort) ApplyBoardOp(_ context.Context, op board.Op) (board.Receipt, error) {
	f.applied = append(f.applied, op)
	if f.reject != nil {
		return board.Receipt{}, f.reject
	}
	return board.Receipt{Seq: uint64(len(f.applied)), Duplicate: f.duplicate}, nil
}

func (f *fakeBoardPort) BoardView(time.Time) (agentbus.View, error) { return f.view, nil }

func (f *fakeBoardPort) BoardIdentity() (string, string, error) {
	if f.identityErr != nil {
		return "", "", f.identityErr
	}
	return f.participant, f.dir, nil
}

// The talk seam: this fake records what the tool asked it to say, so the argument wiring is
// pinned here and the delivery itself by the effect guard in internal/boot.
func (f *fakeBoardPort) AskBoard(_ context.Context, topic, to, text string) (string, error) {
	f.asked = append(f.asked, strings.Join([]string{topic, to, text}, "|"))
	return "correlation-1", nil
}

func (f *fakeBoardPort) AnswerBoard(_ context.Context, correlation, topic, to, text string) (uint64, error) {
	f.answered = append(f.answered, strings.Join([]string{correlation, topic, to, text}, "|"))
	return uint64(len(f.answered)), nil
}

// The deliberation seam: this fake records what the tool asked the hearing log to do
// and hands back the record a host would have produced, so the argument wiring is
// pinned here and the delivery by the effect guard in internal/boot.
func (f *fakeBoardPort) OpenHearing(_ context.Context, node string, required []string) (agentbus.HearingRecord, error) {
	f.hearing = append(f.hearing, strings.Join([]string{"open", node, strings.Join(required, ",")}, "|"))
	return f.hearingRec, f.hearingErr
}

func (f *fakeBoardPort) AnswerHearing(_ context.Context, node, text string, evidence []board.Evidence) (agentbus.HearingRecord, error) {
	refs := make([]string, 0, len(evidence))
	for _, item := range evidence {
		refs = append(refs, item.Ref)
	}
	f.hearing = append(f.hearing, strings.Join([]string{"answer", node, text, strings.Join(refs, ",")}, "|"))
	return f.hearingRec, f.hearingErr
}

func (f *fakeBoardPort) SettleHearing(_ context.Context, node string) (agentbus.HearingRecord, error) {
	f.hearing = append(f.hearing, "settle|"+node)
	return f.hearingRec, f.hearingErr
}

func boardArgs(t *testing.T, raw string) json.RawMessage {
	t.Helper()
	if !json.Valid([]byte(raw)) {
		t.Fatalf("test args are not JSON: %s", raw)
	}
	return json.RawMessage(raw)
}

// An assertion carries the claim it makes: the board keeps that as the assertion's summary,
// so a reader sees why the step is believed to hold instead of only that somebody said so.
func TestAgentBusToolAssertCarriesItsReason(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
	tool := NewAgentBusTool(port)
	if _, err := tool.Execute(context.Background(), boardArgs(t,
		`{"action":"assert","node":"build","reason":"tests pass on the shipped path","evidence":[{"ref":"go test ./..."}]}`)); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if len(port.applied) != 1 {
		t.Fatalf("applied %d ops, want one", len(port.applied))
	}
	if got := port.applied[0].Reason; got != "tests pass on the shipped path" {
		t.Fatalf("assertion reason = %q, want the claim the caller stated", got)
	}
}

// The same call can name the node it lays down. Without that a deliverable root reaches the
// panel as a bare id, which is what forced the orchestration spec onto require/split.
func TestAgentBusToolAssertNamesTheNodeItCreates(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
	tool := NewAgentBusTool(port)
	if _, err := tool.Execute(context.Background(), boardArgs(t,
		`{"action":"assert","node":"build","title":"Ship the parser","evidence":[{"ref":"go test ./..."}]}`)); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if len(port.applied) != 1 {
		t.Fatalf("applied %d ops, want one", len(port.applied))
	}
	if got := port.applied[0].Title; got != "Ship the parser" {
		t.Fatalf("assert title = %q, want the name the caller stated", got)
	}
}

// The tool is registered for every session, so the session without a board is the
// common case: it has to say why, in terms the user's own UI uses.
func TestAgentBusToolRefusesWithoutABoardRatherThanFailing(t *testing.T) {
	tool := NewAgentBusTool(&fakeBoardPort{participant: "alice"})
	for _, action := range []string{"view", `assert`} {
		args := `{"action":"` + action + `"}`
		if action == "assert" {
			args = `{"action":"assert","node":"build","evidence":[{"ref":"go test ./..."}]}`
		}
		_, err := tool.Execute(context.Background(), boardArgs(t, args))
		if err == nil {
			t.Fatalf("%s with no board must be refused", action)
		}
		if !strings.Contains(err.Error(), "not on a board") || !strings.Contains(err.Error(), "collaboration entry") {
			t.Fatalf("%s error = %q, want it to point at the collaboration entry", action, err)
		}
	}
	if _, err := NewAgentBusTool(nil).Execute(context.Background(), boardArgs(t, `{"action":"view"}`)); err == nil {
		t.Fatal("an unbound tool must refuse too, not panic")
	}
}

func TestAgentBusToolMapsEveryActionToABoardOp(t *testing.T) {
	cases := []struct {
		name  string
		args  string
		verb  board.Verb
		node  string
		check func(t *testing.T, op board.Op)
	}{
		{
			name: "assert",
			args: `{"action":"assert","node":"build","evidence":[{"kind":"test","ref":"go test ./...","note":"green"}]}`,
			verb: board.VerbAssert, node: "build",
			check: func(t *testing.T, op board.Op) {
				if len(op.Evidence) != 1 || op.Evidence[0].Ref != "go test ./..." || op.Evidence[0].Kind != "test" {
					t.Fatalf("evidence = %+v, want the call's ref", op.Evidence)
				}
			},
		},
		{
			name: "claim",
			args: `{"action":"claim","node":"build","steps":3,"tokens":1000,"output":"a binary","leaseSeconds":60}`,
			verb: board.VerbClaim, node: "build",
			check: func(t *testing.T, op board.Op) {
				if op.Bounds == nil || op.Bounds.Steps != 3 || op.Bounds.Tokens != 1000 || op.Bounds.Output != "a binary" {
					t.Fatalf("bounds = %+v, want the declared bounds", op.Bounds)
				}
				want := time.Now().UTC().Add(60 * time.Second)
				if op.Deadline.Before(want.Add(-time.Minute)) || op.Deadline.After(want.Add(time.Minute)) {
					t.Fatalf("deadline = %s, want about %s", op.Deadline, want)
				}
			},
		},
		{
			name: "claim defaults its lease",
			args: `{"action":"claim","node":"build","steps":1}`,
			verb: board.VerbClaim, node: "build",
			check: func(t *testing.T, op board.Op) {
				want := time.Now().UTC().Add(agentBusDefaultLease)
				if op.Deadline.Before(want.Add(-time.Minute)) || op.Deadline.After(want.Add(time.Minute)) {
					t.Fatalf("deadline = %s, want the documented default lease", op.Deadline)
				}
			},
		},
		{name: "heartbeat", args: `{"action":"heartbeat","node":"build","leaseSeconds":120}`, verb: board.VerbHeartbeat, node: "build"},
		{name: "release", args: `{"action":"release","node":"build"}`, verb: board.VerbRelease, node: "build"},
		{
			name: "decide",
			args: `{"action":"decide","node":"build","outcome":"done","reproducedBy":"bob","evidence":[{"ref":"ci/run/42"}]}`,
			verb: board.VerbDecide, node: "build",
			check: func(t *testing.T, op board.Op) {
				if op.Outcome != board.OutcomeDone || op.ReproducedBy != "bob" {
					t.Fatalf("op = %+v, want done reproduced by bob", op)
				}
			},
		},
		{name: "refute", args: `{"action":"refute","node":"build","reason":"the test misses the migration"}`, verb: board.VerbRefute, node: "build"},
		{
			name: "assign",
			args: `{"action":"assign","node":"build","assignee":"bob"}`,
			verb: board.VerbAssign, node: "build",
			check: func(t *testing.T, op board.Op) {
				if op.Assignee != "bob" {
					t.Fatalf("assignee = %q, want the call's participant", op.Assignee)
				}
			},
		},
		{
			name: "unassign",
			args: `{"action":"unassign","node":"build"}`,
			verb: board.VerbAssign, node: "build",
			check: func(t *testing.T, op board.Op) {
				if op.Assignee != "" {
					t.Fatalf("assignee = %q, want the node back in the pool", op.Assignee)
				}
			},
		},
		{
			name: "split",
			args: `{"action":"split","node":"root","children":[{"id":"a","title":"first"},{"id":"b"}]}`,
			verb: board.VerbSplit, node: "root",
			check: func(t *testing.T, op board.Op) {
				if len(op.Children) != 2 || op.Children[0].Title != "first" {
					t.Fatalf("children = %+v, want both children with titles", op.Children)
				}
			},
		},
		{
			name: "require",
			args: `{"action":"require","node":"build","dep":{"id":"key","title":"signing key"}}`,
			verb: board.VerbRequire, node: "build",
			check: func(t *testing.T, op board.Op) {
				if op.Dep == nil || op.Dep.ID != "key" || op.Dep.Title != "signing key" {
					t.Fatalf("dep = %+v, want the dependency spec", op.Dep)
				}
			},
		},
		{name: "capability_gap", args: `{"action":"capability_gap","node":"build","reason":"需要签名工具"}`, verb: board.VerbCapabilityGap, node: "build"},
		{name: "abandon", args: `{"action":"abandon","node":"build","reason":"superseded","evidence":[{"ref":"decision/9"}]}`, verb: board.VerbAbandon, node: "build"},
		{name: "revert", args: `{"action":"revert","node":"build"}`, verb: board.VerbRevert, node: "build"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
			out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, tc.args))
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(port.applied) != 1 {
				t.Fatalf("applied %d ops, want exactly 1", len(port.applied))
			}
			op := port.applied[0]
			if op.Verb != tc.verb || op.Node != tc.node {
				t.Fatalf("op = %s on %q, want %s on %q", op.Verb, op.Node, tc.verb, tc.node)
			}
			if op.Actor != "alice" {
				t.Fatalf("actor = %q, want the session's own identity", op.Actor)
			}
			if tc.check != nil {
				tc.check(t, op)
			}
			if !strings.Contains(out, string(tc.verb)) || !strings.Contains(out, "seq 1") {
				t.Fatalf("result = %q, want the verb and the recorded sequence", out)
			}
		})
	}
}

// The model must not be able to claim work without declaring what it costs, and the
// kernel's own reason set is what the caller is told to fix.
func TestAgentBusToolNamesWhatAnIncompleteCallIsMissing(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
	cases := []struct{ name, args, want string }{
		{"claim without bounds", `{"action":"claim","node":"build"}`, "steps"},
		{"decide without outcome", `{"action":"decide","node":"build"}`, "outcome"},
		{"split without children", `{"action":"split","node":"root"}`, "children"},
		{"require without dep", `{"action":"require","node":"build"}`, "dep"},
		{"assign without assignee", `{"action":"assign","node":"build"}`, "assign needs assignee"},
		{"unknown action", `{"action":"approve","node":"build"}`, "unknown action"},
		{"no action", `{}`, "action is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, tc.args))
			if err == nil {
				t.Fatal("the call must be refused before it reaches the board")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to name %q", err, tc.want)
			}
			if len(port.applied) != 0 {
				t.Fatalf("a refused call must not reach the board: %+v", port.applied)
			}
		})
	}
}

// A board refusal is the board's answer: the model reads the reason and corrects the
// op, so it comes back as tool text rather than as a failed call.
func TestAgentBusToolReportsABoardRefusalAsText(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "alice",
		reject: &board.RejectError{Verb: board.VerbDecide, Node: "build", Reason: board.ReasonMissingEvidence},
	}
	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"decide","node":"build","outcome":"done"}`))
	if err != nil {
		t.Fatalf("a refusal must not surface as a tool error: %v", err)
	}
	if !strings.Contains(out, board.ReasonMissingEvidence) || !strings.Contains(out, "evidence") {
		t.Fatalf("result = %q, want the reason and what would fix it", out)
	}

	port.reject = errors.New("disk on fire")
	if _, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"decide","node":"build","outcome":"done"}`)); err == nil {
		t.Fatal("a real failure must still be an error")
	}
}

func TestAgentBusToolReportsAReplayAsAlreadyRecorded(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice", duplicate: true}
	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"release","node":"build"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "already recorded") {
		t.Fatalf("result = %q, want the replay to say nothing changed", out)
	}
}

// action=view is the read the model plans from: it carries the board, the identity, and
// the lines the kernel allows this participant to see.
func TestAgentBusToolViewRendersTheParticipantsOwnView(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/home/u/.reasonix/agentbus/default", participant: "alice",
		view: agentbus.View{
			Board: "default", Participant: "alice", Next: 7, Owned: 1,
			Lines: []agentbus.ViewLine{{ID: "build", State: board.StateClaimed, LastSeq: 4}},
		},
	}
	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"view"}`))
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	for _, want := range []string{"board default", "as alice", "build", "claimed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view = %q, want it to contain %q", out, want)
		}
	}
	if len(port.applied) != 0 {
		t.Fatalf("a read must not write an op: %+v", port.applied)
	}
}

// A hearing is how a contested node stops being contested: the tool has to be able to
// start one, or a refutation on a board nobody can click leaves the work stalled (G1).
func TestAgentBusToolOpensADeliberationAndNamesWhoMustAnswer(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "alice",
		hearingRec: agentbus.HearingRecord{Node: "design", Kind: agentbus.HearingOpen, Required: []string{"bob", "carol"}},
	}
	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"hearing_open","node":"design"}`))
	if err != nil {
		t.Fatalf("hearing_open: %v", err)
	}
	// An empty required list means "whoever must answer": the host decides, and the
	// tool reports the list it actually got back.
	if len(port.hearing) != 1 || port.hearing[0] != "open|design|" {
		t.Fatalf("hearing calls = %+v, want the empty required list handed to the host", port.hearing)
	}
	for _, want := range []string{"design", "bob", "carol"} {
		if !strings.Contains(out, want) {
			t.Fatalf("result = %q, want it to name %q", out, want)
		}
	}
}

func TestAgentBusToolPassesTheNamedAnswerersAndDropsBlanks(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
	_, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t,
		`{"action":"hearing_open","node":"design","required":["bob",""," alice "]}`))
	if err != nil {
		t.Fatalf("hearing_open: %v", err)
	}
	if len(port.hearing) != 1 || port.hearing[0] != "open|design|bob,alice" {
		t.Fatalf("hearing calls = %+v, want the named answerers, trimmed and without blanks", port.hearing)
	}
}

func TestAgentBusToolAnsweringADeliberationNeedsTextAndCarriesEvidence(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
	tool := NewAgentBusTool(port)
	if _, err := tool.Execute(context.Background(), boardArgs(t, `{"action":"hearing_answer","node":"design"}`)); err == nil {
		t.Fatal("an answer with no text says nothing, which is not an answer")
	}
	if len(port.hearing) != 0 {
		t.Fatalf("a refused call must not reach the hearing log: %+v", port.hearing)
	}
	if _, err := tool.Execute(context.Background(), boardArgs(t,
		`{"action":"hearing_answer","node":"design","text":"it does not hold","evidence":[{"kind":"test","ref":"go test ./..."}]}`)); err != nil {
		t.Fatalf("hearing_answer: %v", err)
	}
	if len(port.hearing) != 1 || port.hearing[0] != "answer|design|it does not hold|go test ./..." {
		t.Fatalf("hearing calls = %+v, want the answer with its checkable reference", port.hearing)
	}
}

func TestAgentBusToolReportingAVerdictSaysWhatItWas(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "alice",
		hearingRec: agentbus.HearingRecord{Node: "design", Verdict: agentbus.VerdictRefuted, Reason: agentbus.ReasonWeight},
	}
	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"hearing_settle","node":"design"}`))
	if err != nil {
		t.Fatalf("hearing_settle: %v", err)
	}
	if len(port.hearing) != 1 || port.hearing[0] != "settle|design" {
		t.Fatalf("hearing calls = %+v, want the settle forwarded", port.hearing)
	}
	for _, want := range []string{"design", agentbus.VerdictRefuted, agentbus.ReasonWeight} {
		if !strings.Contains(out, want) {
			t.Fatalf("result = %q, want it to contain %q", out, want)
		}
	}
}

// Every deliberation action names the node it is about: without one there is nothing to
// open, answer or settle, and the caller gets told rather than a hearing about "".
func TestAgentBusToolRefusesADeliberationWithNoNode(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
	for _, action := range []string{"hearing_open", "hearing_answer", "hearing_settle"} {
		if _, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t,
			`{"action":"`+action+`"}`)); err == nil {
			t.Fatalf("%s with no node must be refused", action)
		}
	}
	if len(port.hearing) != 0 {
		t.Fatalf("nothing may reach the hearing log: %+v", port.hearing)
	}
}
