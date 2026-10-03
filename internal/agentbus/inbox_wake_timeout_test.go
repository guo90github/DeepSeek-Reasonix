package agentbus

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A host wakes people once per tick, so a peer that accepts the connection and then never
// answers must not be waited on forever. DeliverWake bounds its own call rather than trusting
// every call site to hand it a deadline.
func TestADeliveryToAPeerThatNeverAnswersGivesUp(t *testing.T) {
	stuck := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-stuck
	}))
	defer server.Close()
	defer close(stuck) // LIFO: unblock the peer before waiting for the server to shut down

	restore := wakeDeliveryTimeout
	wakeDeliveryTimeout = 200 * time.Millisecond
	defer func() { wakeDeliveryTimeout = restore }()

	start := time.Now()
	err := DeliverWake(context.Background(), nil, WakeDelivery{
		BaseURL: server.URL,
		Token:   "token",
	}, WakeMessage{Prompt: "the wake", Display: "the wake", Key: "wake:1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the bound to end the wait rather than hang", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("gave up after %s, want the bound to bite", elapsed)
	}
}

// The bound is on the wait, not on the peer: a host that answers in time still lands the wake,
// in the shape the receiving inbox reads (addressed by the session header, collapsed by the
// key inside the body).
func TestADeliveryToAPeerThatAnswersLands(t *testing.T) {
	type seen struct {
		body   map[string]string
		auth   string
		header string
	}
	gotc := make(chan seen, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var decoded map[string]string
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Errorf("decode body: %v", err)
		}
		gotc <- seen{body: decoded, auth: r.Header.Get("Authorization"), header: r.Header.Get(SessionPathHeader)}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	if err := DeliverWake(context.Background(), nil, WakeDelivery{
		BaseURL:       server.URL,
		Token:         "token",
		SessionHeader: SessionPathHeader,
		SessionPath:   "/someone/session.jsonl",
	}, WakeMessage{Prompt: "the wake", Display: "the line", Key: "wake:1"}); err != nil {
		t.Fatalf("DeliverWake: %v", err)
	}

	got := <-gotc
	if got.auth != "Bearer token" {
		t.Fatalf("authorization = %q, want the announced token", got.auth)
	}
	if got.header != "/someone/session.jsonl" {
		t.Fatalf("session header = %q, want the addressed session", got.header)
	}
	want := map[string]string{"input": "the wake", "display": "the line", "intent": "followup", "idempotencyKey": "wake:1"}
	if !maps.Equal(got.body, want) {
		t.Fatalf("body = %v, want %v", got.body, want)
	}
}
