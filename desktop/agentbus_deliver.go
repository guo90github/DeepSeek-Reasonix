package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"reasonix/internal/agentbus"
)

// agentBusInboxPath is the endpoint a wake is delivered to on another host: the same
// one a person's POST lands on, so a woke session is indistinguishable from one
// somebody prompted (AGENT_BUS §3.2).
const agentBusInboxPath = "/inbox/items"

// AgentBusDelivery is where one wake goes when the participant is not in this
// process: the host's base URL, the bearer token that host published, and the
// session header it reads to know which session the wake is for.
type AgentBusDelivery struct {
	BaseURL string
	Token   string
	// SessionHeader names the addressed-session header the target host reads. It is
	// passed in rather than hardcoded: the header belongs to the serve endpoint.
	SessionHeader string
	// SessionPath addresses one session. Empty lands in that host's foreground, which
	// is why an addressed wake is preferred.
	SessionPath string
}

// deliverAgentBusWake posts a wake to another host. The idempotency key is the wake's
// own key, so a retried delivery collapses on the receiving side exactly as a local
// one does.
func deliverAgentBusWake(ctx context.Context, client *http.Client, delivery AgentBusDelivery, target agentbus.WakeTarget) error {
	base := strings.TrimSpace(delivery.BaseURL)
	if base == "" {
		return fmt.Errorf("desktop: no host to deliver the wake for %q", target.Participant)
	}
	if strings.TrimSpace(delivery.Token) == "" {
		return fmt.Errorf("desktop: refusing to deliver a wake for %q without a token", target.Participant)
	}
	text := agentBusWakePrompt(target)
	payload, err := json.Marshal(map[string]string{
		"input":          text,
		"display":        text,
		"intent":         "followup",
		"idempotencyKey": target.Key,
	})
	if err != nil {
		return fmt.Errorf("desktop: encode wake: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(base, "/")+agentBusInboxPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("desktop: build wake request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(delivery.Token))
	if header := strings.TrimSpace(delivery.SessionHeader); header != "" && strings.TrimSpace(delivery.SessionPath) != "" {
		request.Header.Set(header, delivery.SessionPath)
	}

	client = clientOr(client)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("desktop: deliver wake for %q: %w", target.Participant, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("desktop: wake for %q refused with %s: %s",
			target.Participant, response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func clientOr(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return http.DefaultClient
}
