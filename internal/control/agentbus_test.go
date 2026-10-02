package control

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/event"
)

func newAgentBusTestController(t *testing.T) *Controller {
	t.Helper()
	sessionDir := t.TempDir()
	c := New(Options{SessionDir: sessionDir, Label: "test", Sink: event.Discard})
	c.SetSessionPath(filepath.Join(sessionDir, "session.jsonl"))
	return c
}

func applyBusOps(t *testing.T, dir string, ops ...board.Op) {
	t.Helper()
	b, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	if _, err := b.ApplyAll(context.Background(), ops...); err != nil {
		t.Fatalf("apply ops: %v", err)
	}
}

func busAssert(node, actor string) board.Op {
	return board.Op{
		Verb: board.VerbAssert, Node: node, Actor: actor,
		Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:" + node}},
	}
}

func TestAgentBusIsOptedOutByDefault(t *testing.T) {
	c := newAgentBusTestController(t)
	if block := c.agentBusTurnBlock(); block != "" {
		t.Fatalf("an unwired controller must compose unchanged, got %q", block)
	}
	if _, ok := c.AgentBusView(time.Now().UTC()); ok {
		t.Fatalf("an unwired controller has no view")
	}
}

func TestAgentBusTurnBlockCarriesOnlyMyViewAndAdvancesTheCursor(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTestController(t)
	participant := c.parentSessionID()
	if strings.TrimSpace(participant) == "" {
		t.Skip("controller has no session identity to address on a board")
	}
	applyBusOps(t, dir,
		busAssert("mine", participant),
		busAssert("theirs", "someone-else"),
	)
	c.SetAgentBus(dir, "")

	first := c.agentBusTurnBlock()
	if first == "" {
		t.Fatal("the view must reach the turn once the board is wired")
	}
	if !strings.Contains(first, "node id=mine") {
		t.Fatalf("my own node is missing from the turn block:\n%s", first)
	}
	if strings.Contains(first, "node id=theirs") {
		t.Fatalf("another participant's node leaked into my turn:\n%s", first)
	}
	if !strings.Contains(first, "schema=agentbus-view/1") {
		t.Fatalf("the block must carry the schema header:\n%s", first)
	}

	if second := c.agentBusTurnBlock(); second != "" {
		t.Fatalf("the cursor must suppress a repeat delivery, got %q", second)
	}

	applyBusOps(t, dir, busAssert("fresh", participant))
	third := c.agentBusTurnBlock()
	if !strings.Contains(third, "node id=fresh") {
		t.Fatalf("a new op must arrive next turn:\n%s", third)
	}
	if strings.Contains(third, "node id=mine") {
		t.Fatalf("the delta must not repeat what was already delivered:\n%s", third)
	}

	view, ok := c.AgentBusView(time.Now().UTC())
	if !ok || view.Owned != 2 {
		t.Fatalf("peek view = %+v ok=%v, want two owned nodes", view, ok)
	}
}

func TestSetAgentBusDirEmptyOptsOut(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTestController(t)
	applyBusOps(t, dir, busAssert("mine", c.parentSessionID()))
	c.SetAgentBus(dir, "")
	if c.agentBusTurnBlock() == "" {
		t.Fatal("wired board should produce a block")
	}
	c.SetAgentBus("", "")
	if block := c.agentBusTurnBlock(); block != "" {
		t.Fatalf("opting out must stop the block, got %q", block)
	}
	if _, ok := c.AgentBusView(time.Now().UTC()); ok {
		t.Fatal("opting out must drop the view")
	}
}

func TestUnreadableBoardLeavesTheTurnIntact(t *testing.T) {
	c := newAgentBusTestController(t)
	c.SetAgentBus(filepath.Join(t.TempDir(), "missing-as-a-file"), "")
	if block := c.agentBusTurnBlock(); block != "" {
		t.Fatalf("a board that cannot be read must contribute nothing, got %q", block)
	}
}

func TestAgentBusWritePathAndHumanRows(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTestController(t)
	c.SetAgentBus(dir, "alice")
	bg := context.Background()
	if _, err := c.ApplyAgentBusOp(bg, busAssert("design", "alice")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	rows, ok := c.AgentBusTasks(time.Now().UTC())
	if !ok || len(rows) != 1 || rows[0].ID != "design" || !rows[0].Ready {
		t.Fatalf("rows = %+v ok=%v, want one ready node", rows, ok)
	}
	if !strings.Contains(c.agentBusTurnBlock(), "node id=design") {
		t.Fatal("a written op must reach its author's next turn")
	}
}

func TestAgentBusRefusalIsTyped(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTestController(t)
	c.SetAgentBus(dir, "alice")
	bg := context.Background()
	if _, err := c.ApplyAgentBusOp(bg, busAssert("n", "alice")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	_, err := c.ApplyAgentBusOp(bg, board.Op{Verb: board.VerbClaim, Node: "n", Actor: "alice", Deadline: time.Now().UTC().Add(time.Hour)})
	if reason, ok := board.IsReject(err); !ok || reason != board.ReasonMissingBounds {
		t.Fatalf("refusal = (%q, %v), want missing_bounds", reason, ok)
	}
}

func TestAgentBusUnwiredWriteFails(t *testing.T) {
	c := newAgentBusTestController(t)
	if _, err := c.ApplyAgentBusOp(context.Background(), busAssert("n", "alice")); err == nil {
		t.Fatal("an unenrolled controller must refuse to write")
	}
	if _, ok := c.AgentBusTasks(time.Now().UTC()); ok {
		t.Fatal("an unenrolled controller has no human rows")
	}
}

// A host that serves its own sessions publishes where they speak from, and a session
// that leaves stops being an address others keep waking.
func TestAgentBusAnnouncePublishesAndWithdrawsThisHostsAddress(t *testing.T) {
	c := newAgentBusTestController(t)
	dir := t.TempDir()
	tokenFile := filepath.Join(t.TempDir(), "serve.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("token file: %v", err)
	}
	c.SetAgentBus(dir, "alice")

	if err := c.AgentBusAnnounce("http://127.0.0.1:8977", tokenFile); err != nil {
		t.Fatalf("announce: %v", err)
	}
	directory, err := agentbus.OpenParticipantDirectory(dir)
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	ref, ok, err := directory.Lookup("alice")
	if err != nil || !ok {
		t.Fatalf("lookup = (%+v, %v, %v), want the announced address", ref, ok, err)
	}
	if ref.Host != "http://127.0.0.1:8977" || ref.TokenFile != tokenFile || ref.SessionPath != c.SessionPath() {
		t.Fatalf("ref = %+v, want this host's endpoint and session", ref)
	}

	if err := c.AgentBusWithdraw(); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if _, ok, err := directory.Lookup("alice"); err != nil || ok {
		t.Fatalf("lookup after withdraw = (%v, %v), want the address gone", ok, err)
	}
}

// An unenrolled controller has no address to publish: announcing must say so instead of
// writing an address for a board this session is not on.
func TestAgentBusAnnounceNeedsABoard(t *testing.T) {
	c := newAgentBusTestController(t)
	if err := c.AgentBusAnnounce("http://127.0.0.1:1", ""); err == nil {
		t.Fatal("an unenrolled controller must refuse to announce")
	}
	if err := c.AgentBusWithdraw(); err == nil {
		t.Fatal("an unenrolled controller has nothing to withdraw")
	}
}
