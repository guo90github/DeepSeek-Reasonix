package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// Room-side frozen sample: cmd/chatting/testdata/room_line_receipt_sample.json.
const roomLineSamplePath = "cmd/chatting/testdata/room_line_receipt_sample.json"

// The room froze what it read; this replays what is replayable against a real host
// and says plainly what is not. The frozen host_answer was authored by the room's
// stub, so its gate is the stub's word rather than this host's — that half can only
// be checked as a shape, never as an oracle. What a real host can be held to is the
// input half plus the sentence it relays verbatim.
func TestRoomLineSampleAgreesWithThisHost(t *testing.T) {
	repo := os.Getenv("CHATTING_REPO")
	if repo == "" {
		t.Log("未启用跨仓比对：设置 CHATTING_REPO 指向 chatting 检出来开启")
		return
	}
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(roomLineSamplePath)))
	if err != nil {
		t.Logf("未启用跨仓比对：读不到样本（%v）", err)
		return
	}
	var sample struct {
		Input struct {
			Seq            int64  `json:"seq"`
			Source         string `json:"source"`
			Refusal        string `json:"refusal"`
			IdempotencyKey string `json:"idempotencyKey"`
		} `json:"input"`
		HostAnswer map[string]any `json:"host_answer"`
	}
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("样本解不动：%v", err)
	}
	if sample.Input.Seq <= 0 || sample.Input.IdempotencyKey == "" || sample.Input.Source == "" {
		t.Fatalf("样本 input 半不完整：%+v", sample.Input)
	}

	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	t.Cleanup(c.Close)

	if _, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentSteer, Source: sample.Input.Source,
		Submit: "点名", Display: "点名", Raw: "点名",
		Extra:       map[string]string{"room.seq": strconv.FormatInt(sample.Input.Seq, 10)},
		Idempotency: sample.Input.IdempotencyKey,
	}); err != nil {
		t.Fatalf("EnqueueInbox: %v", err)
	}

	refusal, _ := sample.HostAnswer["reason"].(string)
	resumable, _ := sample.HostAnswer["resumable"].(bool)
	if refusal != "" {
		items := c.InboxSnapshot().Items
		if len(items) != 1 {
			t.Fatalf("队列里有 %d 条，想要 1 条", len(items))
		}
		c.noteInboxHostAnswer(items[0].ID, &InboxDispatchRefusal{Reason: refusal, Resumable: resumable})
	}

	line, found := c.InboxRoomLineFor(sample.Input.Seq)
	if want, _ := sample.HostAnswer["found"].(bool); found != want {
		t.Fatalf("found = %v，样本说 %v", found, want)
	}
	if !found {
		return
	}
	if want, _ := sample.HostAnswer["state"].(string); line.State != want {
		t.Fatalf("state = %q，样本说 %q", line.State, want)
	}
	if line.Source != sample.Input.Source {
		t.Fatalf("source = %q，样本 input 说 %q", line.Source, sample.Input.Source)
	}
	if line.Reason != refusal {
		t.Fatalf("reason = %q，宿主原话 %q 未被逐字带出", line.Reason, refusal)
	}
	if line.Resumable != resumable {
		t.Fatalf("resumable = %v，样本说 %v", line.Resumable, resumable)
	}
	// Wall-clock derived: never frozen. A queued line must report a wait; how long
	// it is belongs to the moment it is asked.
	if line.State == string(sessioninbox.StateQueued) && line.QueuedForMs <= 0 {
		t.Fatalf("queuedForMs = %d，排队的行必须报出等待时长", line.QueuedForMs)
	}

	// Shape contract: every field the room froze must exist here, or a reader of one
	// side sees a field the other never sends.
	emitted := map[string]bool{
		"found": true, "state": true, "source": true, "gate": true,
		"reason": true, "resumable": true, "queuedForMs": true,
	}
	for key := range sample.HostAnswer {
		if !emitted[key] {
			t.Fatalf("样本里有 %q，宿主这一版不发这个字段", key)
		}
	}
}
