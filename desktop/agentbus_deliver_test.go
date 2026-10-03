package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

func TestDeliverAgentBusWakeSpeaksTheServeContract(t *testing.T) {
	type received struct {
		path    string
		auth    string
		session string
		body    map[string]string
	}
	got := make(chan received, 1)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		got <- received{
			path:    r.URL.Path,
			auth:    r.Header.Get("Authorization"),
			session: r.Header.Get("X-Session-Path"),
			body:    body,
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer host.Close()

	target := agentbus.WakeTarget{Participant: "bob", Key: "agentbus-wake:bob:deadbeef", Ready: []string{"schema"}}
	err := deliverAgentBusWake(context.Background(), host.Client(), AgentBusDelivery{
		BaseURL: host.URL, Token: "secret", SessionHeader: "X-Session-Path", SessionPath: "/tmp/bob.jsonl",
	}, target)
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	request := <-got
	if request.path != "/inbox/items" {
		t.Fatalf("path = %q, want the inbox endpoint", request.path)
	}
	if request.auth != "Bearer secret" {
		t.Fatalf("authorization = %q, want the host's bearer token", request.auth)
	}
	if request.session != "/tmp/bob.jsonl" {
		t.Fatalf("session header = %q, want the addressed session", request.session)
	}
	if request.body["idempotencyKey"] != target.Key {
		t.Fatalf("idempotencyKey = %q, want the wake's own key so a retry collapses", request.body["idempotencyKey"])
	}
	if request.body["intent"] != "followup" {
		t.Fatalf("intent = %q, want a follow-up turn", request.body["intent"])
	}
	// The two fields are different by design: the model reads the block, a person reads the
	// line. They matched before wakes started showing a human sentence, so this is about the
	// contract rather than the two fields happening to agree.
	if request.body["input"] == request.body["display"] {
		t.Fatalf("input and display are the same text (%q); the model reads the block and a person reads the line", request.body["input"])
	}
	if !strings.Contains(request.body["input"], "<agentbus-wake>") || !strings.Contains(request.body["input"], "schema") {
		t.Fatalf("input = %q, want the block that says why the session is being woken", request.body["input"])
	}
	if strings.Contains(request.body["display"], "<agentbus-wake>") {
		t.Fatalf("display = %q, want a line for a person rather than the XML block", request.body["display"])
	}
}

func TestDeliverAgentBusWakeRefusesSilentMisroutes(t *testing.T) {
	target := agentbus.WakeTarget{Participant: "bob", Key: "k"}
	if err := deliverAgentBusWake(context.Background(), nil, AgentBusDelivery{Token: "t"}, target); err == nil {
		t.Fatal("a wake with no host must be refused, not dropped")
	}
	if err := deliverAgentBusWake(context.Background(), nil, AgentBusDelivery{BaseURL: "http://127.0.0.1:1"}, target); err == nil {
		t.Fatal("a wake with no token must be refused before it leaves the process")
	}

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "addressed session cannot receive this here", http.StatusConflict)
	}))
	defer host.Close()
	err := deliverAgentBusWake(context.Background(), host.Client(), AgentBusDelivery{BaseURL: host.URL, Token: "t"}, target)
	if err == nil {
		t.Fatal("a refusal from the target host must surface, not be reported as delivered")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Fatalf("err = %v, want the host's status in it", err)
	}
}
