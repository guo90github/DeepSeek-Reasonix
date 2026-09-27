package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
)

// The origin and the terminal state of a wake item have to be readable off the
// wire, not inferred from a log line: without them the room cannot tell a wake
// that was applied from one that never landed, and reports "never woken".
func TestInboxItemEndpointReportsOriginAndTerminalState(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	posted := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer"}`, "")
	if posted.StatusCode != http.StatusAccepted {
		t.Fatalf("enqueue: status=%d, want 202", posted.StatusCode)
	}
	receipt := decodeInboxReceipt(t, posted)

	response, err := http.Get(httpServer.URL + "/inbox/items/" + receipt.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Meta struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			State  string `json:"state"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode item view: %v (%s)", err, body)
	}
	if view.Meta.ID != receipt.ItemID {
		t.Fatalf("item id = %q, want %q", view.Meta.ID, receipt.ItemID)
	}
	if view.Meta.Source != "http" {
		t.Fatalf("source = %q, want http", view.Meta.Source)
	}
	if view.Meta.State == "" {
		t.Fatalf("terminal state is missing: %s", body)
	}
	t.Logf("GET /inbox/items/%s -> %s", receipt.ItemID, strings.TrimSpace(string(body)))
}

// A room that names its own line number on the wake gets it back on the item, so
// the two sides reconcile on the line rather than on a host-only id. The field is
// additive: without it the item is the same plain http wake as before.
func TestInboxItemEndpointKeepsTheRoomLineItWasGiven(t *testing.T) {
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	posted := postInboxWake(t, httpServer.URL, `{"input":"room wake","intent":"steer","seq":"43"}`, "")
	if posted.StatusCode != http.StatusAccepted {
		t.Fatalf("enqueue: status=%d, want 202", posted.StatusCode)
	}
	receipt := decodeInboxReceipt(t, posted)

	response, err := http.Get(httpServer.URL + "/inbox/items/" + receipt.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Meta struct {
			Source string `json:"source"`
			Room   *struct {
				Seq int64 `json:"seq"`
			} `json:"room"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode item view: %v (%s)", err, body)
	}
	if view.Meta.Source != "room-wake" {
		t.Fatalf("source = %q, want room-wake for an item naming a room line", view.Meta.Source)
	}
	if view.Meta.Room == nil || view.Meta.Room.Seq != 43 {
		t.Fatalf("room line = %+v, want seq 43 (body: %s)", view.Meta.Room, body)
	}
}
