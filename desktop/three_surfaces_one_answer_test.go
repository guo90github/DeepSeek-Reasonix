package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/serve"
	"reasonix/internal/sessioninbox"
)

// One room line, three surfaces: the controller query, the serve endpoint and the
// desktop bridge must answer the same thing — including how the line ended. Any
// surface that computes the answer for itself drifts here.
//
// Scope: the two endings a caller outside internal/control can produce (cancelled,
// removed) plus a seq nobody took. `acknowledged` is written when a turn finishes,
// which only internal/control can drive, so that ending is pinned there
// (TestRoomLineEndingsAreRememberedBySeq) and reaches these surfaces unmodified.
func TestEverySurfaceAnswersOneRoomLineTheSame(t *testing.T) {
	isolateDesktopUserDirs(t)
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctrl := control.New(control.Options{SessionPath: session, SessionDir: dir, Label: "foreground", Sink: event.Discard})
	t.Cleanup(ctrl.Close)
	httpServer := httptest.NewServer(serve.New(ctrl, nil, config.ServeConfig{}).Handler())
	defer httpServer.Close()

	app := &App{
		tabs:        map[string]*WorkspaceTab{"tab-a": {ID: "tab-a", Scope: "global", Ready: true, Ctrl: ctrl}},
		tabOrder:    []string{"tab-a"},
		activeTabID: "tab-a",
	}

	enqueue := func(seq int64) string {
		t.Helper()
		rec, err := ctrl.EnqueueInbox(control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "desktop:tab-a",
			Submit: "点名", Display: "点名", Raw: "点名",
			Extra:       map[string]string{"room.seq": strconv.FormatInt(seq, 10)},
			Idempotency: "room-wake:chatting:127.0.0.1:8899:" + strconv.FormatInt(seq, 10),
		})
		if err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		return rec.ItemID
	}

	cancelled := enqueue(60)
	if _, err := ctrl.CancelWithInboxItemsResult([]string{cancelled}, "desktop:tab-a"); err != nil {
		t.Fatalf("CancelWithInboxItemsResult: %v", err)
	}
	removed := enqueue(61)
	if err := ctrl.DeleteInboxItem(removed); err != nil {
		t.Fatalf("DeleteInboxItem: %v", err)
	}

	type answer struct {
		found   bool
		state   string
		settled string
	}
	ask := func(seq int64) answer {
		t.Helper()
		line, found := ctrl.InboxRoomLineFor(seq)
		fromControl := answer{found, line.State, line.Settled}

		response, err := http.Get(httpServer.URL + "/inbox/room-line?seq=" + strconv.FormatInt(seq, 10))
		if err != nil {
			t.Fatalf("GET /inbox/room-line: %v", err)
		}
		defer response.Body.Close()
		var wire struct {
			Found bool                   `json:"found"`
			Line  *control.InboxRoomLine `json:"line"`
		}
		if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
			t.Fatalf("decode endpoint answer: %v", err)
		}
		fromEndpoint := answer{wire.Found, "", ""}
		if wire.Line != nil {
			fromEndpoint.state, fromEndpoint.settled = wire.Line.State, wire.Line.Settled
		}

		view, err := app.InboxRoomLine("tab-a", seq)
		if err != nil {
			t.Fatalf("App.InboxRoomLine: %v", err)
		}
		fromBridge := answer{view.Found, "", ""}
		if view.Line != nil {
			fromBridge.state, fromBridge.settled = view.Line.State, view.Line.Settled
		}

		if fromControl != fromEndpoint || fromControl != fromBridge {
			t.Fatalf("seq %d: control=%+v endpoint=%+v bridge=%+v，三个面必须答同一件事", seq, fromControl, fromEndpoint, fromBridge)
		}
		return fromControl
	}

	for _, want := range []struct {
		seq     int64
		settled string
	}{
		{60, "discarded"},
		{61, "deleted"},
		{62, ""},
	} {
		got := ask(want.seq)
		if got.found || got.settled != want.settled {
			t.Fatalf("seq %d: got %+v，想要 found=false settled=%q", want.seq, got, want.settled)
		}
	}
}
