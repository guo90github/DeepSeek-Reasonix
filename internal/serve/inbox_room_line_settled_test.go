package serve

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
	"reasonix/internal/sessioninbox"
)

// The ledger is only useful if a sender can read it: the by-seq answer must carry
// how the line ended, for each way it left the queue. This pins the last link —
// ledger to HTTP — for a cancelled and a removed line.
func TestRoomLineEndingsReachTheWire(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "room-line.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sink := &runtimeStateServeSink{states: make(chan event.RuntimeStateSnapshot, 64)}
	foreground := control.New(control.Options{SessionDir: dir, SessionPath: path, Label: "foreground", Sink: sink})
	t.Cleanup(foreground.Close)
	server := New(foreground, nil, config.ServeConfig{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	enqueue := func(seq int64, idempotency string) sessioninbox.InboxReceipt {
		t.Helper()
		rec, err := foreground.EnqueueInbox(control.InboxRequest{
			Intent: sessioninbox.IntentSteer, Source: "push",
			Submit: "点名", Display: "点名", Raw: "点名",
			Extra:       map[string]string{"room.seq": strconv.FormatInt(seq, 10)},
			Idempotency: idempotency,
		})
		if err != nil {
			t.Fatalf("EnqueueInbox: %v", err)
		}
		return rec
	}

	askSeq := func(seq int64) map[string]any {
		t.Helper()
		response, err := http.Get(httpServer.URL + "/inbox/room-line?seq=" + strconv.FormatInt(seq, 10))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		body := map[string]any{}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body
	}

	// Removed outright.
	if err := foreground.DeleteInboxItem(enqueue(61, "room-wake:chatting:127.0.0.1:8899:61").ItemID); err != nil {
		t.Fatalf("DeleteInboxItem: %v", err)
	}
	body := askSeq(61)
	if found, _ := body["found"].(bool); found {
		t.Fatalf("body = %+v，删掉的那条不该还在队列里", body)
	}
	line, _ := body["line"].(map[string]any)
	if line == nil || line["settled"] != "deleted" {
		t.Fatalf("body = %+v，线上该说出 deleted", body)
	}
	if _, ok := line["settledAt"]; !ok {
		t.Fatalf("body = %+v，线上该带上收尾时刻", body)
	}

	// Cancelled while still pending.
	cancelled := enqueue(62, "room-wake:chatting:127.0.0.1:8899:62")
	if _, err := foreground.CancelWithInboxItemsResult([]string{cancelled.ItemID}, "push"); err != nil {
		t.Fatalf("CancelWithInboxItemsResult: %v", err)
	}
	line, _ = askSeq(62)["line"].(map[string]any)
	if line == nil || line["settled"] != "discarded" {
		t.Fatalf("line = %+v，线上该说出 discarded", line)
	}

	// A seq nobody took in says nothing about endings.
	body = askSeq(63)
	if found, _ := body["found"].(bool); found {
		t.Fatalf("body = %+v", body)
	}
	if _, ok := body["line"]; ok {
		t.Fatalf("body = %+v，没接过的 seq 不该带 line", body)
	}
}
