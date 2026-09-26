package boot

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"reasonix/internal/plugin"
)

// roomWakeFixturePath is the frozen producer payload both repositories keep:
// the host asserts its mapping against this file instead of a second
// hand-written copy, which is where the two sides used to drift.
const roomWakeFixturePath = "testdata/room_wake_payload.json"

// roomWakeProducerFixture is the same file's path inside the chatting checkout,
// beside the payload's own producer (cmd/chatting).
const roomWakeProducerFixture = "cmd/chatting/testdata/room_wake_payload.json"

// roomWakeFrozenFields is the contract: a payload that gained, lost, or renamed
// a field is a change on the producing side and must be carried here on purpose.
var roomWakeFrozenFields = []string{"from", "kind", "mentions", "origin", "panel", "seq", "text", "topic"}

func readRoomWakeFixture(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(roomWakeFixturePath)
	if err != nil {
		t.Fatalf("read %s: %v", roomWakeFixturePath, err)
	}
	return json.RawMessage(raw)
}

// roomWakeFieldMismatch reports the fixture's fields when they differ from the
// frozen contract, and nothing when they match.
func roomWakeFieldMismatch(raw json.RawMessage) ([]string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("payload is not a JSON object: %w", err)
	}
	got := make([]string, 0, len(payload))
	for key := range payload {
		got = append(got, key)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(roomWakeFrozenFields, ",") {
		return got, nil
	}
	return nil, nil
}

// A payload change on either side must fail here instead of passing silently:
// the frozen field set is what the host's room.* mapping is written against.
func TestRoomWakeFixturePinsTheFrozenFieldSet(t *testing.T) {
	got, err := roomWakeFieldMismatch(readRoomWakeFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("fixture fields = %v, want the frozen contract %v", got, roomWakeFrozenFields)
	}
}

// The guard above is only worth its green if it bites: this is the same check
// run against a payload that lost a field, and it must report the change.
func TestRoomWakeFieldSetGuardRejectsAPayloadChange(t *testing.T) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(readRoomWakeFixture(t), &payload); err != nil {
		t.Fatal(err)
	}
	delete(payload, "mentions")
	payload["mentionsText"] = json.RawMessage(`"renamed"`)
	perturbed, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := roomWakeFieldMismatch(perturbed)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("a payload with a renamed field passed the frozen-contract check")
	}
}

// Every payload field the producer sends must reach its room.* key, and the
// idempotency key must stay pinned to the room's own identity — both read from
// the fixture, so a mapping change in this repository fails here.
func TestRoomWakeFixtureMapsToRoomMetaAndIdempotency(t *testing.T) {
	raw := readRoomWakeFixture(t)
	var payload struct {
		Seq      int64    `json:"seq"`
		From     string   `json:"from"`
		Text     string   `json:"text"`
		Topic    int64    `json:"topic"`
		Mentions []string `json:"mentions"`
		Kind     string   `json:"kind"`
		Origin   string   `json:"origin"`
		Panel    string   `json:"panel"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}

	body, extra, idem, err := wakeInbox(plugin.WakeMessage{
		Server: "chatting", Method: "notifications/chatting/room_message", Payload: raw,
	})
	if err != nil {
		t.Fatalf("wakeInbox: %v", err)
	}
	if body != strings.TrimSpace(payload.Text) {
		t.Fatalf("body = %q, want the fixture's text verbatim", body)
	}
	want := map[string]string{
		"room.seq":      strconv.FormatInt(payload.Seq, 10),
		"room.from":     payload.From,
		"room.topic":    strconv.FormatInt(payload.Topic, 10),
		"room.kind":     payload.Kind,
		"room.origin":   payload.Origin,
		"room.mentions": strings.Join(payload.Mentions, "\n"),
		"room.panel":    payload.Panel,
	}
	for key, value := range want {
		if extra[key] != value {
			t.Fatalf("extra[%s] = %q, want %q", key, extra[key], value)
		}
	}
	u, err := url.Parse(payload.Panel)
	if err != nil || u.Host == "" {
		t.Fatalf("fixture panel %q is not a usable room identity", payload.Panel)
	}
	if want := "room-wake:chatting:" + u.Host + ":" + strconv.FormatInt(payload.Seq, 10); idem != want {
		t.Fatalf("idempotency = %q, want %q", idem, want)
	}
}

// The producing repository owns the bytes: with CHATTING_REPO set, the two
// copies must be identical so a payload change made there fails here. Unset says
// so instead of skipping in silence.
func TestRoomWakeFixtureMatchesTheProducerCopy(t *testing.T) {
	repo := strings.TrimSpace(os.Getenv("CHATTING_REPO"))
	if repo == "" {
		t.Log("CHATTING_REPO is not set, so the cross-repository byte comparison did not run; set it to the chatting checkout to enable it")
		return
	}
	producer := filepath.Join(repo, filepath.FromSlash(roomWakeProducerFixture))
	theirs, err := os.ReadFile(producer)
	if err != nil {
		t.Fatalf("read the producer's fixture %s: %v", producer, err)
	}
	if string(theirs) != string(readRoomWakeFixture(t)) {
		t.Fatalf("%s and %s differ: the payload was changed on one side only", producer, roomWakeFixturePath)
	}
}
