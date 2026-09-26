package plugin

import (
	"context"
	"encoding/json"
	"testing"
)

type wakeTransport struct {
	notifications notificationRouter
	closed        bool
}

func (t *wakeTransport) call(context.Context, string, any) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (t *wakeTransport) close() { t.closed = true }

func (t *wakeTransport) registerNotification(method string, callback func(json.RawMessage)) func() {
	return t.notifications.registerNotification(method, callback)
}

func newWakeClient(spec Spec) (*Client, *wakeTransport) {
	tr := &wakeTransport{}
	return &Client{name: spec.Name, t: tr, spec: spec}, tr
}

const testWakeMethod = "notifications/room/room_message"

// A server that declared a wake method reaches the host sink with its own
// name, method, and payload. This is the whole point of the channel: work
// arrives while the session is not calling room_wait.
func TestDeclaredWakeMethodReachesTheHostSink(t *testing.T) {
	host := NewHostWithProfile(HostProfileCore)
	var got []WakeMessage
	host.SetWakeHandler(func(msg WakeMessage) WakeOutcome {
		got = append(got, msg)
		return WakeDelivered
	})

	client, tr := newWakeClient(Spec{Name: "room", WakeMethod: testWakeMethod})
	host.bindWakeNotifications(client)

	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{"seq":43}`))
	if len(got) != 1 {
		t.Fatalf("wakes delivered = %d, want 1", len(got))
	}
	if got[0].Server != "room" || got[0].Method != testWakeMethod {
		t.Fatalf("wake = %+v, want server=room method=%s", got[0], testWakeMethod)
	}
	if string(got[0].Payload) != `{"seq":43}` {
		t.Fatalf("payload = %s", got[0].Payload)
	}
}

// Connecting is not a grant: a server that declared no wake method cannot wake
// anything, and its notification is simply not routed.
func TestUndeclaredServerCannotWakeTheSession(t *testing.T) {
	host := NewHostWithProfile(HostProfileCore)
	delivered := 0
	host.SetWakeHandler(func(WakeMessage) WakeOutcome {
		delivered++
		return WakeDelivered
	})

	client, tr := newWakeClient(Spec{Name: "room"})
	host.bindWakeNotifications(client)

	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{"seq":43}`))
	if delivered != 0 {
		t.Fatalf("an undeclared server woke the session %d time(s)", delivered)
	}
}

// Closing the client unarms the wake method: a stale registration must not let
// a torn-down server start work afterwards.
func TestClosedClientNoLongerWakesTheSession(t *testing.T) {
	host := NewHostWithProfile(HostProfileCore)
	delivered := 0
	host.SetWakeHandler(func(WakeMessage) WakeOutcome {
		delivered++
		return WakeDelivered
	})

	client, tr := newWakeClient(Spec{Name: "room", WakeMethod: testWakeMethod})
	host.bindWakeNotifications(client)
	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{}`))
	if delivered != 1 {
		t.Fatalf("wakes before close = %d, want 1", delivered)
	}

	client.close()
	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{}`))
	if delivered != 1 {
		t.Fatalf("wakes after close = %d, want the one from before", delivered)
	}
}

// The wake method is host-only policy: it must not move the provider-visible
// schema cache key, or arming one server would churn every session's cache.
func TestSchemaCacheKeyIgnoresWakeMethod(t *testing.T) {
	spec := sampleSpec()
	want := SchemaCacheKey(spec)
	spec.WakeMethod = testWakeMethod
	if got := SchemaCacheKey(spec); got != want {
		t.Fatalf("host-only wake method changed the schema cache key: got %q want %q", got, want)
	}
}

// A wake carries the session whose call is the only identity the child could
// have learned. Without it a host serving several sessions has nothing to route
// by, and the desktop's shared host drops the wake instead of delivering it.
func TestWakeCarriesTheCallerSession(t *testing.T) {
	host := NewHostWithProfile(HostProfileCore)
	var got []WakeMessage
	host.SetWakeHandler(func(msg WakeMessage) WakeOutcome {
		got = append(got, msg)
		return WakeDelivered
	})

	client, tr := newWakeClient(Spec{Name: "room", WakeMethod: testWakeMethod})
	host.bindWakeNotifications(client)
	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{"seq":43}`))
	if len(got) != 1 || got[0].Caller != "" {
		t.Fatalf("wake = %+v, want no caller before any call", got)
	}

	client.noteCallerSession("/sessions/room.jsonl")
	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{"seq":44}`))
	if len(got) != 2 || got[1].Caller != "/sessions/room.jsonl" {
		t.Fatalf("wake = %+v, want caller=/sessions/room.jsonl", got)
	}
}

// What the sink did with a wake is a fact about the server, so the host keeps it:
// "the room pushed and nothing happened" must be answerable from the server's own
// status, not by guessing from a log line.
func TestWakeOutcomeIsKeptPerServer(t *testing.T) {
	host := NewHostWithProfile(HostProfileCore)
	host.SetWakeHandler(func(WakeMessage) WakeOutcome { return WakeRefused })

	client, tr := newWakeClient(Spec{Name: "room", WakeMethod: testWakeMethod})
	host.bindWakeNotifications(client)
	if got := host.WakeOutcomeFor("room"); got != "" {
		t.Fatalf("outcome before any wake = %q, want empty", got)
	}

	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{"seq":43}`))
	if got := host.WakeOutcomeFor("room"); got != WakeRefused {
		t.Fatalf("outcome = %q, want %q", got, WakeRefused)
	}
	for _, s := range host.Servers() {
		if s.Name == "room" && s.LastWake != string(WakeRefused) {
			t.Fatalf("server status LastWake = %q, want %q", s.LastWake, WakeRefused)
		}
	}

	client.close()
	if got := host.WakeOutcomeFor("room"); got != WakeRefused {
		t.Fatalf("outcome after the client closed = %q, want the recorded answer", got)
	}
}

// With no sink installed the drop is still an outcome the host can report: an
// unwired channel must not look the same as a delivered wake.
func TestWakeWithoutHandlerRecordsTheDrop(t *testing.T) {
	host := NewHostWithProfile(HostProfileCore)
	client, tr := newWakeClient(Spec{Name: "room", WakeMethod: testWakeMethod})
	host.bindWakeNotifications(client)

	tr.notifications.dispatchNotification(testWakeMethod, json.RawMessage(`{"seq":43}`))
	if got := host.WakeOutcomeFor("room"); got != WakeNoHandler {
		t.Fatalf("outcome = %q, want %q", got, WakeNoHandler)
	}
}
