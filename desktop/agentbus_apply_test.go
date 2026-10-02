package main

import (
	"strings"
	"testing"
)

// The panel's write path is the model's tool, not a second implementation: what the
// human sends has to be what the board validates, refusals included.
func TestAgentBusApplyWritesThroughTheSameToolTheModelUses(t *testing.T) {
	app, ctrl, home := agentBusEnrolApp(t)
	if _, err := app.AgentBusJoin(); err != nil {
		t.Fatalf("join: %v", err)
	}

	out, err := app.AgentBusApply(AgentBusApplyArgs{
		Action: "assert", Node: "build",
		Reason: "the build must be reproducible", Ref: "go test ./...",
	})
	if err != nil {
		t.Fatalf("assert: %v", err)
	}
	if !strings.Contains(out, "assert") || !strings.Contains(out, "recorded") {
		t.Fatalf("assert answer = %q, want the recorded op reported", out)
	}

	// A refusal is the board's answer and has to reach the panel as text, not as a
	// failure: the human reads what the board wants changed.
	refused, err := app.AgentBusApply(AgentBusApplyArgs{Action: "decide", Node: "build", Outcome: "done"})
	if err != nil {
		t.Fatalf("a refusal must not surface as a call failure: %v", err)
	}
	if !strings.Contains(refused, "refused") {
		t.Fatalf("decide without evidence = %q, want a refusal naming the reason", refused)
	}

	// The board really holds it: the session's own view comes back with the node.
	view, err := app.AgentBusApply(AgentBusApplyArgs{Action: "view"})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if !strings.Contains(view, "build") {
		t.Fatalf("view = %q, want the node the human just recorded", view)
	}
	if !strings.Contains(home, "") {
		t.Fatalf("home = %q", home)
	}
	if ctrl.AgentBusDir() == "" {
		t.Fatal("the session must stay enrolled after a human write")
	}
}

func TestAgentBusApplyRefusesWhenTheSessionJoinedNothing(t *testing.T) {
	app, _, _ := agentBusEnrolApp(t)
	// The tool refuses a session with no board before it touches one, and the panel
	// shows that message: the human reads "join a board", not an empty success.
	_, err := app.AgentBusApply(AgentBusApplyArgs{Action: "assert", Node: "build", Ref: "go test ./..."})
	if err == nil {
		t.Fatal("an action on a session that joined nothing must be refused")
	}
	if !strings.Contains(err.Error(), "not on a board") || !strings.Contains(err.Error(), "collaboration entry") {
		t.Fatalf("error = %q, want it to say where a board is joined", err)
	}
	if _, err := (&App{}).AgentBusApply(AgentBusApplyArgs{Action: "view"}); err == nil {
		t.Fatal("no active session must be reported")
	}
}

// The panel's flat form has to reach the board as the arguments the kernel validates.
func TestAgentBusApplyPayloadCarriesTheBoardsOwnArguments(t *testing.T) {
	require := agentBusToolPayload(AgentBusApplyArgs{
		Action: "require", Node: "build", Dep: &AgentBusChildArg{ID: "key", Title: "signing key"},
	})
	dep, ok := require["dep"].(map[string]string)
	if !ok || dep["id"] != "key" || dep["title"] != "signing key" {
		t.Fatalf("dep = %v, want the dependency the board expects", require["dep"])
	}

	split := agentBusToolPayload(AgentBusApplyArgs{
		Action: "split", Node: "root",
		Children: []AgentBusChildArg{{ID: "a", Title: "first"}, {ID: "b"}},
	})
	children, ok := split["children"].([]map[string]string)
	if !ok || len(children) != 2 || children[1]["id"] != "b" {
		t.Fatalf("children = %v, want both children", split["children"])
	}
	if _, titled := children[1]["title"]; titled {
		t.Fatalf("a child without a title must not invent one: %v", children[1])
	}

	decide := agentBusToolPayload(AgentBusApplyArgs{
		Action: "decide", Node: "build", Outcome: "done", Ref: "ci/7", ReproducedBy: "bob",
	})
	evidence, ok := decide["evidence"].([]map[string]string)
	if !ok || len(evidence) != 1 || evidence[0]["ref"] != "ci/7" {
		t.Fatalf("evidence = %v, want the ref the human typed", decide["evidence"])
	}
	if decide["outcome"] != "done" || decide["reproducedBy"] != "bob" {
		t.Fatalf("payload = %v, want the outcome and the re-runner", decide)
	}

	claim := agentBusToolPayload(AgentBusApplyArgs{Action: "claim", Node: "x"})
	if _, present := claim["steps"]; present {
		t.Fatal("a claim without steps must let the tool refuse, not send a fabricated bound")
	}
}
