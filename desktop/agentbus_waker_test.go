package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

func agentBusTab(t *testing.T, dir, id, participant string) *WorkspaceTab {
	t.Helper()
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	ctrl.SetAgentBus(dir, participant)
	return &WorkspaceTab{ID: id, Ctrl: ctrl}
}

func TestAgentBusWakeRecipientFindsTheOwningTab(t *testing.T) {
	dir := t.TempDir()
	app := &App{tabs: map[string]*WorkspaceTab{
		"t1": agentBusTab(t, dir, "t1", "alice"),
		"t2": agentBusTab(t, dir, "t2", "bob"),
	}}

	if tab, err := app.agentBusWakeRecipient("bob"); err != nil || tab == nil || tab.ID != "t2" {
		t.Fatalf("recipient = %+v (err %v), want the tab that speaks as bob", tab, err)
	}
	if tab, err := app.agentBusWakeRecipient("nobody"); err != nil || tab != nil {
		t.Fatalf("recipient = %+v (err %v), want none for a participant no tab owns", tab, err)
	}
	if tab, err := app.agentBusWakeRecipient("   "); err != nil || tab != nil {
		t.Fatal("an empty participant must route to nobody rather than to the first tab")
	}
}

// Two tabs speaking as one participant must be reported, not resolved by luck: waking the wrong
// session is the failure this routing exists to prevent ("别搞错会话", 2026-10-03).
func TestAgentBusWakeRecipientReportsTwoTabsSpeakingAsOneParticipant(t *testing.T) {
	dir := t.TempDir()
	app := &App{tabs: map[string]*WorkspaceTab{
		"t1": agentBusTab(t, dir, "t1", "bob"),
		"t2": agentBusTab(t, dir, "t2", "bob"),
	}}

	tab, err := app.agentBusWakeRecipient("bob")
	if err == nil {
		t.Fatalf("recipient = %+v, want the ambiguity reported rather than one of two tabs", tab)
	}
}

func TestRouteAgentBusWakeReportsAnUnreachableParticipant(t *testing.T) {
	app := &App{tabs: map[string]*WorkspaceTab{}}
	err := app.routeAgentBusWakeOn(context.Background(), "", agentbus.WakeTarget{Participant: "ghost", Key: "k"})
	if err == nil {
		t.Fatal("a wake no tab owns must be reported, not dropped silently")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("err = %v, want it to name the participant", err)
	}
}

func TestAgentBusWakePromptCarriesTheReason(t *testing.T) {
	prompt := agentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob",
		Ready:       []string{"schema"},
		Waiting:     []string{"design"},
		Asks:        []string{"ask-1"},
		Owes:        []string{"dispute"},
	})
	for _, want := range []string{"<agentbus-wake>", "schema", "design", "ask-1", "dispute"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt is missing %q:\n%s", want, prompt)
		}
	}
	if empty := agentBusWakePrompt(agentbus.WakeTarget{Participant: "bob"}); strings.Contains(empty, "startable now") {
		t.Fatalf("a wake with nothing to show must not invent a list:\n%s", empty)
	}
}

// A dispatched assignment is not a wake: the host already claimed the node in the
// participant's name, so the message says what the recipient owns.
func TestDispatchMessageNamesTheAssignmentInsteadOfAWake(t *testing.T) {
	dispatch := agentbus.WakeTarget{
		Participant: "bob",
		Key:         agentbus.DispatchKey("default", "schema"),
		Ready:       []string{"schema"},
	}
	prompt := agentBusWakePrompt(dispatch)
	if !strings.Contains(prompt, "assigned this work to you: schema") {
		t.Fatalf("the dispatch prompt must name the assignment:\n%s", prompt)
	}
	if strings.Contains(prompt, "woke you") {
		t.Fatalf("the dispatch prompt must not read as a plain wake:\n%s", prompt)
	}
	if line := agentBusWakeLine(dispatch); line != "已指派给你：schema" {
		t.Fatalf("dispatch line = %q, want the assignment named to a person", line)
	}

	wake := agentbus.WakeTarget{Participant: "bob", Key: "agentbus-wake:default/bob", Ready: []string{"schema"}}
	if !strings.HasPrefix(agentBusWakeLine(wake), "The board has work for you") {
		t.Fatalf("a plain wake keeps its wording, got %q", agentBusWakeLine(wake))
	}
	if !strings.Contains(agentBusWakePrompt(wake), "woke you") {
		t.Fatalf("a plain wake keeps its prompt:\n%s", agentBusWakePrompt(wake))
	}
}

func TestWakeIsDeliveredToTheHostTheDirectoryNames(t *testing.T) {
	boardDir := t.TempDir()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	type seen struct {
		auth    string
		session string
		body    string
	}
	got := make(chan seen, 1)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- seen{
			auth:    r.Header.Get("Authorization"),
			session: r.Header.Get(agentBusSessionHeader),
			body:    string(body),
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer host.Close()

	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	if _, err := directory.Announce(context.Background(), agentbus.ParticipantRef{
		Participant: "bob", Host: host.URL, SessionPath: "/sessions/bob.jsonl", TokenFile: tokenFile,
	}); err != nil {
		t.Fatalf("announce: %v", err)
	}

	// No local tab owns bob: the wake must travel to the host the board's address book
	// names, addressed to his session, carrying that host's token.
	app := &App{tabs: map[string]*WorkspaceTab{}}
	target := agentbus.WakeTarget{Participant: "bob", Key: "agentbus-wake:bob:aa", Ready: []string{"schema"}}
	if err := app.routeAgentBusWakeOn(context.Background(), boardDir, target); err != nil {
		t.Fatalf("route: %v", err)
	}
	request := <-got
	if request.auth != "Bearer secret" {
		t.Fatalf("authorization = %q, want the announced host's token", request.auth)
	}
	if request.session != "/sessions/bob.jsonl" {
		t.Fatalf("session header = %q, want the address book's session", request.session)
	}
	if !strings.Contains(request.body, target.Key) || !strings.Contains(request.body, "schema") {
		t.Fatalf("body = %s, want the wake's key and reason", request.body)
	}
}

func TestWakeWithoutAnAddressIsRefusedNotDropped(t *testing.T) {
	app := &App{tabs: map[string]*WorkspaceTab{}}
	target := agentbus.WakeTarget{Participant: "nobody", Key: "k"}
	if err := app.routeAgentBusWakeOn(context.Background(), t.TempDir(), target); err == nil {
		t.Fatal("an unaddressed wake must be refused, not dropped silently")
	}
}
