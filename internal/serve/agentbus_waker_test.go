package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/boot"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

func agentBusSession(t *testing.T, boardDir, participant string) *control.Controller {
	t.Helper()
	// A real session path: an inbox item belongs to a session, and serve's controllers
	// always have one.
	sessionDir := t.TempDir()
	ctrl := control.New(control.Options{
		SessionDir:  sessionDir,
		SessionPath: filepath.Join(sessionDir, "session.jsonl"),
		Sink:        event.Discard,
	})
	ctrl.SetAgentBus(boardDir, participant)
	// The inbox keeps files open under the session dir; on Windows that makes the temporary
	// directory unremovable unless the session is closed first.
	t.Cleanup(ctrl.Close)
	return ctrl
}

// A hosted session is the only host its participant has, so a ready-work wake has to reach
// that session's inbox — otherwise a headless participant moves only when somebody else writes.
func TestHostedSessionReceivesAWakeForItsOwnParticipant(t *testing.T) {
	bob := agentBusSession(t, t.TempDir(), "bob")
	s := &Server{tags: map[*control.Controller]*sessionTagSink{bob: nil}}
	s.installAgentBusWaker(bob)

	target := agentbus.WakeTarget{Participant: "bob", Key: "agentbus-wake:bob:deadbeef", Ready: []string{"schema"}}
	if err := s.deliverAgentBusWake(context.Background(), target); err != nil {
		t.Fatalf("deliver wake: %v", err)
	}
	items := bob.InboxSnapshot().Items
	if len(items) != 1 {
		t.Fatalf("inbox has %d items, want the wake queued", len(items))
	}
	if items[0].Source != control.AgentBusWakeSource || items[0].Idempotency != target.Key {
		t.Fatalf("queued item = %+v, want the wake keyed by its own wake", items[0])
	}
}

// A participant no session here speaks as is a refusal, never a drop: the sender has to see
// that nobody heard, and the host that really owns it delivers for its own participants.
func TestWakeForAParticipantNoSessionSpeaksAsIsRefused(t *testing.T) {
	s := &Server{tags: map[*control.Controller]*sessionTagSink{}}
	err := s.deliverAgentBusWake(context.Background(), agentbus.WakeTarget{Participant: "ghost", Key: "k"})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("err = %v, want a refusal naming the participant", err)
	}
}

// Two sessions claiming one participant is reported rather than guessed: waking the wrong
// session is the failure this routing exists to prevent.
func TestTwoSessionsClaimingOneParticipantAreRefused(t *testing.T) {
	dir := t.TempDir()
	first := agentBusSession(t, dir, "bob")
	second := agentBusSession(t, dir, "bob")
	s := &Server{tags: map[*control.Controller]*sessionTagSink{first: nil, second: nil}}
	if _, err := s.controllerForParticipant("bob"); err == nil || !strings.Contains(err.Error(), "two sessions") {
		t.Fatalf("err = %v, want the ambiguity reported", err)
	}
}

type postedWake struct {
	path    string
	auth    string
	session string
	body    map[string]string
}

// announceOnAnotherHost puts a peer's address in the board's address book and hands back a
// channel carrying whatever that peer's host is asked to deliver.
func announceOnAnotherHost(t *testing.T, boardDir, participant, token string) chan postedWake {
	t.Helper()
	seen := make(chan postedWake, 1)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen <- postedWake{
			path:    r.URL.Path,
			auth:    r.Header.Get("Authorization"),
			session: r.Header.Get(agentbus.SessionPathHeader),
			body:    body,
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(host.Close)

	tokenFile := filepath.Join(t.TempDir(), participant+".token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	directory, err := agentbus.OpenParticipantDirectory(boardDir)
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	if _, err := directory.Announce(context.Background(), agentbus.ParticipantRef{
		Participant: participant,
		Host:        host.URL,
		SessionPath: `C:\elsewhere\` + participant + `-session.jsonl`,
		TokenFile:   tokenFile,
	}); err != nil {
		t.Fatalf("announce %s: %v", participant, err)
	}
	return seen
}

// A participant no session here speaks as is not the end of the road any more: the address book
// says where that host speaks from, and the wake goes there with the token it announced. Without
// this fallback a headless host could only ever be pushed across processes, never push.
func TestAWakeForAParticipantOnAnotherHostGoesToItsAnnouncedAddress(t *testing.T) {
	boardDir := t.TempDir()
	seen := announceOnAnotherHost(t, boardDir, "bob", "bob-announced-token")

	s := &Server{tags: map[*control.Controller]*sessionTagSink{}}
	s.SetControllerBuildOptions(boot.Options{AgentBusDir: boardDir})
	target := agentbus.WakeTarget{Participant: "bob", Key: "agentbus-wake:bob:deadbeef", Ready: []string{"schema"}}
	if err := s.deliverAgentBusWake(context.Background(), target); err != nil {
		t.Fatalf("deliver wake: %v", err)
	}

	got := <-seen
	if got.path != agentbus.InboxItemsPath {
		t.Fatalf("path = %q, want the announced host's inbox endpoint", got.path)
	}
	if got.auth != "Bearer bob-announced-token" {
		t.Fatalf("authorization = %q, want the token bob announced", got.auth)
	}
	if got.session != `C:\elsewhere\bob-session.jsonl` {
		t.Fatalf("session header = %q, want the session bob announced", got.session)
	}
	if got.body["idempotencyKey"] != target.Key || got.body["intent"] != "followup" {
		t.Fatalf("body = %+v, want the wake keyed by its own wake", got.body)
	}
	if !strings.Contains(got.body["input"], "schema") {
		t.Fatalf("input = %q, want the block that tells the session why it was woken", got.body["input"])
	}
}

// The ambiguity guard survives the new fallback: two sessions here claiming one participant must
// not quietly turn into a delivery to somebody else's host — "don't wake the wrong session" is
// the criterion, and an address book gives a wrong answer a place to go.
func TestAnAmbiguousLocalParticipantNeverFallsBackToAnotherHost(t *testing.T) {
	boardDir := t.TempDir()
	seen := announceOnAnotherHost(t, boardDir, "bob", "bob-announced-token")

	first := agentBusSession(t, boardDir, "bob")
	second := agentBusSession(t, boardDir, "bob")
	s := &Server{tags: map[*control.Controller]*sessionTagSink{first: nil, second: nil}}
	s.SetControllerBuildOptions(boot.Options{AgentBusDir: boardDir})

	err := s.deliverAgentBusWake(context.Background(), agentbus.WakeTarget{Participant: "bob", Key: "k"})
	if err == nil || !strings.Contains(err.Error(), "two sessions") {
		t.Fatalf("err = %v, want the ambiguity reported instead of a remote delivery", err)
	}
	select {
	case got := <-seen:
		t.Fatalf("an ambiguous participant was delivered to another host anyway: %+v", got)
	default:
	}
}
