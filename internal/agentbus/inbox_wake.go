package agentbus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// InboxItemsPath is the endpoint a wake is delivered to on another host: the same one a
// person's POST lands on, so a woken session is indistinguishable from one somebody prompted.
const InboxItemsPath = "/inbox/items"

// SessionPathHeader names the addressed session. It is wire protocol between hosts, so it is
// stated once here rather than guessed at each call site.
const SessionPathHeader = "X-Reasonix-Session-Path"

// WakeDelivery is where one wake goes when its participant lives on another host: the base URL
// that host published, the bearer token it announced, and the session header it reads to know
// which session the wake is for. An empty SessionPath lands in that host's foreground.
type WakeDelivery struct {
	BaseURL       string
	Token         string
	SessionHeader string
	SessionPath   string
}

// WakeMessage is what to say: the prompt the woken session reads and the display line a person
// sees, both already rendered by the caller — the kernel does not own session text.
type WakeMessage struct {
	Prompt  string
	Display string
	Key     string
}

// wakeDeliveryTimeout bounds one delivery inside DeliverWake. A host wakes people once per
// tick, so a peer that accepts the connection and then never answers would stall the very loop
// that keeps the board moving; bounding it here covers every caller rather than each call site.
var wakeDeliveryTimeout = 15 * time.Second

// DeliverWake posts one wake to another host's inbox. The idempotency key is the wake's own
// key, so a retried delivery collapses on the receiving side exactly as a local one does, and
// a refusal comes back as an error: a wake nobody received has to be visible to its sender.
// A delivery left unanswered past wakeDeliveryTimeout is given up rather than waited on.
func DeliverWake(ctx context.Context, client *http.Client, delivery WakeDelivery, message WakeMessage) error {
	base := strings.TrimSpace(delivery.BaseURL)
	if base == "" {
		return fmt.Errorf("agentbus: no host to deliver the wake for %q", message.Key)
	}
	if strings.TrimSpace(delivery.Token) == "" {
		return fmt.Errorf("agentbus: refusing to deliver a wake for %q without a token", message.Key)
	}
	ctx, cancel := context.WithTimeout(ctx, wakeDeliveryTimeout)
	defer cancel()
	payload, err := json.Marshal(map[string]string{
		"input":          message.Prompt,
		"display":        message.Display,
		"intent":         "followup",
		"idempotencyKey": message.Key,
	})
	if err != nil {
		return fmt.Errorf("agentbus: encode wake: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(base, "/")+InboxItemsPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("agentbus: build wake request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(delivery.Token))
	if header := strings.TrimSpace(delivery.SessionHeader); header != "" && strings.TrimSpace(delivery.SessionPath) != "" {
		request.Header.Set(header, delivery.SessionPath)
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("agentbus: deliver wake: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("agentbus: wake refused with %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// ReadAnnouncedToken reads the token file a host announced: the secret never travels in the
// address book, only where to find it.
func ReadAnnouncedToken(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("agentbus: the announced address carries no token file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("agentbus: read announced token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
