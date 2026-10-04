package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/tool"
)

func init() { tool.RegisterBuiltin(agentBusBoard{}) }

// errNoBoard is what a session that never joined a board is told. The fix is a UI
// action, so the message names where it lives.
var errNoBoard = errors.New("this session is not on a board: the user joins one from the collaboration entry in the status bar")

// BoardPort is the blackboard as one tool call needs it: the control layer owns the
// enrolment and the board file, so a call forwards one op and reads back the view that
// session is allowed to see. This package never imports the controller (layering:
// internal/tool/builtin must not import internal/control), so a host — where the
// controller lives — supplies the implementation.
type BoardPort interface {
	// BoardIdentity names the board directory and the participant speaking on it. An
	// error is the host's own reason (e.g. the session is not built yet); an empty
	// directory means this session joined nothing.
	BoardIdentity() (participant, boardDir string, err error)
	BoardView(now time.Time) (agentbus.View, error)
	ApplyBoardOp(ctx context.Context, op board.Op) (board.Receipt, error)
	// BoardParticipants lists who is on this board now: the roster is the host's address
	// book, kept beside the board file, and a session that announced itself with no
	// endpoint still belongs on it — that is how a desktop session is visible at all.
	BoardParticipants() ([]agentbus.ParticipantRef, error)
	// BoardPool lists the steps anybody could pick up. Nothing broadcasts them, so a session
	// that wants to help has to be able to ask (F53/F40, 2026-10-05).
	BoardPool() ([]agentbus.PoolEntry, error)
	// AskBoard puts a bounded question to one participant; AnswerBoard answers one that
	// was addressed here. Both write to the board this session is enrolled on.
	AskBoard(ctx context.Context, topic, to, text string) (string, error)
	AnswerBoard(ctx context.Context, correlation, topic, to, text string) (uint64, error)
	// OpenHearing/AnswerHearing/SettleHearing run the deliberation on a contested node.
	// The host owns the hearing log; this package asks for one and reports the record.
	OpenHearing(ctx context.Context, node string, required []string) (agentbus.HearingRecord, error)
	AnswerHearing(ctx context.Context, node, text string, evidence []board.Evidence) (agentbus.HearingRecord, error)
	SettleHearing(ctx context.Context, node string) (agentbus.HearingRecord, error)
}

// NewAgentBusTool binds the blackboard tool to a session's board. A nil port keeps the
// tool in the registry but refusing: the tool list is part of the cache-stable prefix,
// so it must not appear and disappear as a session joins or leaves a board.
func NewAgentBusTool(port BoardPort) tool.Tool { return agentBusBoard{port: port} }

// BindBoardPort rebinds an already-registered board tool, so an assembly that adds
// tools before the controller exists can hand it the port afterwards.
func BindBoardPort(t tool.Tool, port BoardPort) (tool.Tool, bool) {
	b, ok := t.(agentBusBoard)
	if !ok {
		return nil, false
	}
	b.port = port
	return b, true
}

type agentBusBoard struct{ port BoardPort }

func (agentBusBoard) Name() string { return "agent_bus" }

func (agentBusBoard) Description() string {
	return "Shared blackboard for multi-agent work: record what must be true, claim a step before working on it, and let a step be done only with evidence someone else can re-run. One board is shared by every participant, so a node's state — not this conversation — is the source of truth. " +
		"action=view lists the nodes addressed to you; use it before claiming. " +
		"claim requires a deadline and bounds (steps), assert/abandon require evidence, refute and capability_gap require a reason — refute also undoes a done step: the node goes back to contested for a verdict. assign addresses a step to one participant, and only that participant may take it; unassign hands it back to the pool. decide(done) needs an evidenced assert someone else can check plus a reproducer who is not its author; abandon only requests it (an evidenced abandon, then decide(abandoned)); and a blocked step is one you decided against, not one still waiting on a dependency. decide is not ownership-checked: any enrolled participant may close a step out, so close only what its evidence and a reproducer can back. " +
		"A refuted result with nobody to settle it stalls the work: hearing_open starts a deliberation on that node (required= names who must answer; empty means its owner and everyone who refuted it), hearing_answer records your side, hearing_settle records the verdict, and a refuted assertion blocks the node. " +
		"A refusal comes back as a reason (illegal transition, missing evidence, unknown node): read it and fix the op instead of retrying it unchanged."
}

func (agentBusBoard) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "action":{"type":"string","enum":["view","assert","claim","heartbeat","release","decide","refute","split","require","assign","unassign","capability_gap","abandon","revert","ask","answer","hearing_open","hearing_answer","hearing_settle"],"description":"view: read the board as this session is allowed to see it. assert: record something verifiable about a node (creates it if new; pass reason to state the claim in one line). claim: take a step before working on it. release: give it back. decide: the step's outcome (done requires evidence and a reproducer who is not the worker). refute: challenge a result with a reason. split: replace a node with child nodes. require: add a dependency the node waits for. assign: address the node to one participant (set assignee=), who is then the only one that may take it. unassign: hand the node back to the pool. capability_gap: stop and name the capability you lack. abandon: ask for the node to be dropped (needs evidence). revert: undo a done node (its done dependents go stale). ask: put a bounded question to one participant (set to=; it reaches them, it is not a broadcast). answer: answer a question addressed to you (set correlation=). hearing_open: start a deliberation on a contested node (required= names who must answer; empty means its owner and everyone who refuted it). hearing_answer: record your side of the deliberation (text, optional evidence — an answer that brings nothing checkable weighs nothing). hearing_settle: weigh the deliberation and record the verdict; a refuted assertion blocks the node."},
  "node":{"type":"string","description":"Node id. Required for every action except view and split."},
  "title":{"type":"string","description":"Human-readable title; names the node the action creates (require/split children, or the node an assert lays down)."},
  "reason":{"type":"string","description":"Why: assert stores it as the assertion's summary; required by refute, capability_gap and abandon (for capability_gap: what you need, what you tried, why it did not work)."},
  "outcome":{"type":"string","enum":["done","blocked","abandoned"],"description":"decide only."},
  "evidence":{"type":"array","description":"Evidence for assert/abandon and for decide(done): each item needs a ref (command, path, test, URL) that another participant can check.","items":{"type":"object","properties":{"kind":{"type":"string","description":"e.g. test, command, file, url"},"ref":{"type":"string"},"note":{"type":"string"}},"required":["ref"]}},
  "reproducedBy":{"type":"string","description":"decide(done) only: who re-ran the evidence; must not be the participant that produced it."},
  "children":{"type":"array","description":"split only: the child nodes replacing this one.","items":{"type":"object","properties":{"id":{"type":"string"},"title":{"type":"string"}},"required":["id"]}},
  "dep":{"type":"object","description":"require only: the dependency the node waits for.","properties":{"id":{"type":"string"},"title":{"type":"string"}},"required":["id"]},
  "assignee":{"type":"string","description":"assign only: the participant this node is addressed to; only they may take it. Use action=unassign to hand the node back to the pool."},
  "steps":{"type":"integer","description":"claim only: how many steps this work may take (bounds).","minimum":1},
  "tokens":{"type":"integer","description":"claim only: optional token ceiling for the work."},
  "output":{"type":"string","description":"claim only: optional description of what the step produces."},
  "leaseSeconds":{"type":"integer","description":"claim/heartbeat: how long the lease lasts before someone else may take the step (default 900).","minimum":1},
  "topic":{"type":"string","description":"ask/answer only: the conversation surface this belongs to."},
  "to":{"type":"string","description":"ask/answer only: the participant addressed (a question reaches only them)."},
  "text":{"type":"string","description":"ask/answer only: what to say. Speech that names nobody stays out of everyone's context."},
  "correlation":{"type":"string","description":"answer only: the correlation the ask returned."},
  "required":{"type":"array","description":"hearing_open only: who must answer (empty = the node's owner and everyone who refuted it).","items":{"type":"string"}}
},
"required":["action"]
}`)
}

// The board is shared state: a call always reads or writes work other sessions own,
// so it is never parallelised with the rest of a batch.
func (agentBusBoard) ReadOnly() bool { return false }

const agentBusDefaultLease = 900 * time.Second

type agentBusEvidenceArg struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	Note string `json:"note"`
}

type agentBusNodeArg struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type agentBusArgs struct {
	Action       string                `json:"action"`
	Node         string                `json:"node"`
	Assignee     string                `json:"assignee"`
	Title        string                `json:"title"`
	Reason       string                `json:"reason"`
	Outcome      string                `json:"outcome"`
	ReproducedBy string                `json:"reproducedBy"`
	Evidence     []agentBusEvidenceArg `json:"evidence"`
	Children     []agentBusNodeArg     `json:"children"`
	Dep          *agentBusNodeArg      `json:"dep"`
	Steps        int                   `json:"steps"`
	Tokens       int64                 `json:"tokens"`
	Output       string                `json:"output"`
	LeaseSeconds int                   `json:"leaseSeconds"`
	Topic        string                `json:"topic"`
	To           string                `json:"to"`
	Text         string                `json:"text"`
	Correlation  string                `json:"correlation"`
	Required     []string              `json:"required"`
}

func (t agentBusBoard) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in agentBusArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		return "", fmt.Errorf("action is required: one of view, participants, pool, assert, claim, heartbeat, release, decide, refute, split, require, assign, unassign, capability_gap, abandon, revert, ask, answer, hearing_open, hearing_answer, hearing_settle")
	}
	// An item with no ref is dropped by the board, whose refusal then reads "missing_evidence" —
	// as if no evidence had been given at all. Name the empty item instead (2026-10-05).
	for i, item := range in.Evidence {
		if strings.TrimSpace(item.Ref) == "" {
			return "", fmt.Errorf("evidence[%d] has no ref: every evidence item needs the ref that lets another participant check it", i)
		}
	}
	if action == "view" {
		return t.readBoard()
	}
	if action == "participants" {
		return t.readParticipants()
	}
	if action == "pool" {
		return t.readPool()
	}
	if action == "ask" || action == "answer" {
		return t.talk(action, in)
	}
	if action == "hearing_open" || action == "hearing_answer" || action == "hearing_settle" {
		return t.deliberate(action, in)
	}
	actor, _, err := t.identity()
	if err != nil {
		return "", err
	}
	op, err := t.opFor(action, actor, in)
	if err != nil {
		return "", err
	}
	return t.apply(op)
}

// talk is the bounded direct channel: an ask reaches the one participant it names and
// an answer travels back along the correlation the ask returned. Neither is a
// broadcast, which is what keeps speech from becoming a cost bomb.
func (t agentBusBoard) talk(action string, in agentBusArgs) (string, error) {
	if _, _, err := t.identity(); err != nil {
		return "", err
	}
	topic := strings.TrimSpace(in.Topic)
	text := strings.TrimSpace(in.Text)
	if topic == "" || text == "" {
		return "", fmt.Errorf("%s needs topic and text", action)
	}
	if action == "ask" {
		to := strings.TrimSpace(in.To)
		if to == "" {
			return "", fmt.Errorf("ask needs to: a question that names nobody reaches nobody")
		}
		correlation, err := t.port.AskBoard(context.Background(), topic, to, text)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ask recorded on %q for %q (correlation %s): it reaches them, not the board", topic, to, correlation), nil
	}
	correlation := strings.TrimSpace(in.Correlation)
	if correlation == "" {
		return "", fmt.Errorf("answer needs correlation: the value the ask returned")
	}
	seq, err := t.port.AnswerBoard(context.Background(), correlation, topic, strings.TrimSpace(in.To), text)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("answer recorded on %q at seq %d", topic, seq), nil
}

// deliberate is the deliberation chain this session can drive on its own: a refuted
// result is opened up for a verdict, both sides answer, and the verdict is recorded.
// A refusal from the hearing log passes through as the host's own reason.
func (t agentBusBoard) deliberate(action string, in agentBusArgs) (string, error) {
	if _, _, err := t.identity(); err != nil {
		return "", err
	}
	node := strings.TrimSpace(in.Node)
	if node == "" {
		return "", fmt.Errorf("%s needs node: the contested step this deliberation is about", action)
	}
	if action == "hearing_open" {
		record, err := t.port.OpenHearing(context.Background(), node, trimmed(in.Required))
		if err != nil {
			return "", err
		}
		if len(record.Required) == 0 {
			return fmt.Sprintf("hearing opened on %q with nobody required to answer: it can only settle as it stands", node), nil
		}
		return fmt.Sprintf("hearing opened on %q; %s must answer", node, strings.Join(record.Required, ", ")), nil
	}
	if action == "hearing_answer" {
		text := strings.TrimSpace(in.Text)
		if text == "" {
			return "", fmt.Errorf("hearing_answer needs text: what your side of the deliberation is; evidence is what it will weigh")
		}
		if _, err := t.port.AnswerHearing(context.Background(), node, text, evidenceOf(in.Evidence)); err != nil {
			return "", err
		}
		return fmt.Sprintf("answer recorded on %q; it weighs what its evidence can be checked for", node), nil
	}
	record, err := t.port.SettleHearing(context.Background(), node)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("hearing on %q settled: %s (%s)", node, record.Verdict, record.Reason), nil
}

// trimmed drops blank entries, so "required": [""] means "whoever must answer"
// instead of a requirement nobody can satisfy.
func trimmed(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// identity names the board and who is speaking on it. The host's own error passes
// through untouched: it says something this package cannot know.
func (t agentBusBoard) identity() (participant, boardDir string, err error) {
	if t.port == nil {
		return "", "", errNoBoard
	}
	participant, boardDir, err = t.port.BoardIdentity()
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(boardDir) == "" {
		return "", "", errNoBoard
	}
	return strings.TrimSpace(participant), boardDir, nil
}

// readBoard renders the view this participant is allowed to see: owned work, work
// waiting on it, and whatever it asked for. A session with no board says so rather than
// showing an empty one.
func (t agentBusBoard) readBoard() (string, error) {
	participant, boardDir, err := t.identity()
	if err != nil {
		return "", err
	}
	view, err := t.port.BoardView(time.Now().UTC())
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("board %s as %s\n", boardName(boardDir), participant)
	body := strings.TrimSpace(view.Render())
	if body == "" {
		return head + "nothing on this board is addressed to you right now" + t.hereNote(participant), nil
	}
	return head + body + t.hereNote(participant), nil
}

// hereNote points at the company and at the work nobody holds: a session could learn neither
// from a wake (wakes carry work, never company) nor from the roster (only a caller that already
// knew to ask could read it), so the read it already makes is where the pointers belong
// (F53/F40, 2026-10-05). An unreadable roster or board contributes nothing.
func (t agentBusBoard) hereNote(participant string) string {
	return t.peerNote(participant) + t.poolNote()
}

// peerNote is the roster half of hereNote.
func (t agentBusBoard) peerNote(participant string) string {
	refs, err := t.port.BoardParticipants()
	if err != nil {
		return ""
	}
	others := 0
	for _, ref := range refs {
		if ref.Participant != "" && ref.Participant != participant {
			others++
		}
	}
	if others == 0 {
		return ""
	}
	return fmt.Sprintf("\n%d other session(s) on this board (action=participants lists them)", others)
}

// poolNote is the work half of hereNote: the steps anybody could pick up, so that "there is
// something to take" is discoverable from a view rather than only from knowing to ask.
func (t agentBusBoard) poolNote() string {
	entries, err := t.port.BoardPool()
	if err != nil {
		return ""
	}
	if len(entries) == 0 {
		return ""
	}
	return fmt.Sprintf("\n%d step(s) in the pool (action=pool lists them)", len(entries))
}

func (t agentBusBoard) opFor(action, actor string, in agentBusArgs) (board.Op, error) {
	op := board.Op{Verb: board.Verb(action), Node: strings.TrimSpace(in.Node), Actor: actor}
	switch action {
	case "assert":
		op.Evidence = evidenceOf(in.Evidence)
		op.Reason = strings.TrimSpace(in.Reason)
		op.Title = strings.TrimSpace(in.Title)
	case "claim":
		op.Deadline = time.Now().UTC().Add(leaseOf(in.LeaseSeconds))
		bounds := &board.Bounds{Steps: in.Steps, Tokens: in.Tokens, Output: strings.TrimSpace(in.Output)}
		if bounds.Steps <= 0 {
			return board.Op{}, fmt.Errorf("claim needs bounds: pass steps (how many steps this work may take)")
		}
		op.Bounds = bounds
		if title := strings.TrimSpace(in.Title); title != "" {
			op.Reason = title
		}
	case "heartbeat":
		op.Deadline = time.Now().UTC().Add(leaseOf(in.LeaseSeconds))
	case "release":
		op.Verb = board.VerbRelease
	case "decide":
		outcome, err := outcomeOf(in.Outcome)
		if err != nil {
			return board.Op{}, err
		}
		op.Outcome = outcome
		op.Evidence = evidenceOf(in.Evidence)
		op.ReproducedBy = strings.TrimSpace(in.ReproducedBy)
	case "refute", "capability_gap", "abandon":
		op.Reason = strings.TrimSpace(in.Reason)
		op.Evidence = evidenceOf(in.Evidence)
	case "split":
		if len(in.Children) == 0 {
			return board.Op{}, fmt.Errorf("split needs children: at least one {id, title}")
		}
		for _, child := range in.Children {
			if strings.TrimSpace(child.Title) == "" {
				return board.Op{}, fmt.Errorf("split needs a title on every child, including %q: the title is how the child is found later, and no verb renames a node", strings.TrimSpace(child.ID))
			}
		}
		op.Children = specsOf(in.Children)
	case "assign":
		if strings.TrimSpace(in.Assignee) == "" {
			return board.Op{}, fmt.Errorf("assign needs assignee: the participant this node is addressed to; use unassign to return it to the pool")
		}
		op.Assignee = strings.TrimSpace(in.Assignee)
	case "unassign":
		op.Verb = board.VerbUnassign
	case "require":
		if in.Dep == nil || strings.TrimSpace(in.Dep.ID) == "" {
			return board.Op{}, fmt.Errorf("require needs dep: {id, title} the node waits for")
		}
		if strings.TrimSpace(in.Dep.Title) == "" {
			return board.Op{}, fmt.Errorf("require needs a title on the dependency %q: the title is how it is found later, and no verb renames a node", strings.TrimSpace(in.Dep.ID))
		}
		op.Dep = &board.NodeSpec{ID: strings.TrimSpace(in.Dep.ID), Title: strings.TrimSpace(in.Dep.Title)}
	case "revert":
	default:
		return board.Op{}, fmt.Errorf("unknown action %q: one of view, participants, pool, assert, claim, heartbeat, release, decide, refute, split, require, assign, unassign, capability_gap, abandon, revert, ask, answer, hearing_open, hearing_answer, hearing_settle", action)
	}
	return op, nil
}

// apply forwards one op and reports what the board decided. A refusal is the board's
// answer, not a tool failure: it comes back as text so the model can correct the op.
func (t agentBusBoard) apply(op board.Op) (string, error) {
	receipt, err := t.port.ApplyBoardOp(context.Background(), op)
	if err != nil {
		if reason, rejected := board.IsReject(err); rejected {
			return fmt.Sprintf("refused (%s): the board did not accept %s on %q — %s", reason, op.Verb, op.Node, rejectHint(reason, op.Verb)), nil
		}
		// A spending ceiling is a refusal too, and this tool's contract says a refusal comes back
		// as text, not as a failed call. The host keeps its own typed error for the dispatch loop,
		// which is why the conversion lives here at the tool boundary (2026-10-05).
		if reason, refused := agentbus.IsBudgetReject(err); refused {
			return fmt.Sprintf("refused (%s): the spending ceiling turned %s on %q down — the board spent the allowance this step was measured against", reason, op.Verb, op.Node), nil
		}
		return "", err
	}
	if receipt.Duplicate {
		return fmt.Sprintf("%s on %q was already recorded (seq %d); nothing changed", op.Verb, op.Node, receipt.Seq), nil
	}
	return fmt.Sprintf("%s on %q recorded at seq %d", op.Verb, op.Node, receipt.Seq), nil
}

func evidenceOf(items []agentBusEvidenceArg) []board.Evidence {
	if len(items) == 0 {
		return nil
	}
	out := make([]board.Evidence, 0, len(items))
	for _, item := range items {
		ref := strings.TrimSpace(item.Ref)
		if ref == "" {
			continue
		}
		out = append(out, board.Evidence{Kind: strings.TrimSpace(item.Kind), Ref: ref, Note: strings.TrimSpace(item.Note)})
	}
	return out
}

func specsOf(items []agentBusNodeArg) []board.NodeSpec {
	out := make([]board.NodeSpec, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		out = append(out, board.NodeSpec{ID: id, Title: strings.TrimSpace(item.Title)})
	}
	return out
}

func outcomeOf(raw string) (board.Outcome, error) {
	switch board.Outcome(strings.ToLower(strings.TrimSpace(raw))) {
	case board.OutcomeDone, board.OutcomeBlocked, board.OutcomeAbandoned:
		return board.Outcome(strings.ToLower(strings.TrimSpace(raw))), nil
	default:
		return "", fmt.Errorf("decide needs an outcome: done, blocked or abandoned")
	}
}

func leaseOf(seconds int) time.Duration {
	if seconds <= 0 {
		return agentBusDefaultLease
	}
	return time.Duration(seconds) * time.Second
}

// rejectHint turns a board refusal into the correction the caller should make. The
// reasons are the kernel's own closed set; anything new stays unexplained rather than
// guessed at. The verb decides which surface the evidence has to be on: decide checks the
// node's assertions, abandon its own ref (2026-10-05).
func rejectHint(reason string, verb board.Verb) string {
	switch reason {
	case board.ReasonMissingEvidence:
		switch verb {
		case board.VerbDecide:
			return "this node needs an assert carrying evidence first: decide's own refs are not what the board checks here"
		case board.VerbAbandon:
			return "abandon needs a ref of its own — a reason is not evidence: pass evidence with a ref another participant can check"
		}
		return "pass evidence with a ref another participant can check"
	case board.ReasonMissingReason:
		return "pass a reason that says why"
	case board.ReasonMissingDeadline:
		return "claim and heartbeat need a deadline (leaseSeconds)"
	case board.ReasonMissingBounds:
		return "claim needs bounds (steps)"
	case board.ReasonMissingActor:
		return "this session has no identity on the board yet"
	case board.ReasonMissingNode:
		return "name the node this op is about in node"
	case board.ReasonUnknownNode:
		return "no node by that id on this board; create it with require, split or assert"
	case board.ReasonUnknownVerb:
		return "send one of the board's verbs: assert, claim, heartbeat, release, decide, refute, split, require, assign, unassign, capability_gap, abandon, revert"
	case board.ReasonIllegalTransition:
		return "another participant's state does not allow this: read action=view first"
	case board.ReasonNotOwner:
		return "only the claimer may do this: claim the node first, or ask its owner"
	case board.ReasonNotAssignee:
		return "the board addressed this step to someone else: ask for it to be reassigned (assign) or handed back (unassign)"
	case board.ReasonMissingReproducer:
		return "decide(done) needs reproducedBy: name the participant that re-ran the evidence"
	case board.ReasonSelfReproduced:
		return "reproducedBy must not be someone who asserted on this node: ask another participant to re-run it and record that run"
	case board.ReasonMissingAbandonRequest:
		return "decide(abandoned) closes an abandon request that has to exist first: abandon, with evidence"
	case board.ReasonMissingDependency:
		return "require needs dep: {id, title} the node waits for"
	case board.ReasonDuplicateDependency:
		return "this node already waits for that dependency: read its deps with action=view and pick a different one"
	case board.ReasonDuplicateNode:
		return "a split cannot reuse a child id that already exists: read the children it has, then add only the new ones"
	case board.ReasonCycle:
		return "that would make the graph depend on itself: pick a dependency that does not lead back here"
	case board.ReasonDependencyClosed:
		return "that dependency was abandoned, so it can never become done: this node cannot be unblocked by it — abandon or re-scope this node instead"
	case board.ReasonUnknownOutcome:
		return "decide needs an outcome: done, blocked or abandoned"
	case board.ReasonInvalidOutcomeForState:
		return "this outcome does not fit the node's state: read action=view first"
	case board.ReasonDeadlineNotFuture:
		return "the lease has to end in the future: pass a later deadline"
	case board.ReasonSystemOnly:
		return "only the board writes this verb; it reclaims expired claims on its own"
	case board.ReasonIdempotencyConflict:
		return "the same op id was already recorded with a different payload: change the move, not just its arguments"
	case board.ReasonRateLimited:
		return "not now: this node is moving faster than the host allows — send the same move again once the window passes"
	default:
		return "the board's reason is in the message above"
	}
}

func boardName(dir string) string {
	dir = strings.TrimRight(strings.TrimSpace(dir), `/\`)
	if idx := strings.LastIndexAny(dir, `/\`); idx >= 0 {
		return dir[idx+1:]
	}
	return dir
}
