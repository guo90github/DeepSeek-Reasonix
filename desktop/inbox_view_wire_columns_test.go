package main

import (
	"encoding/json"
	"testing"
	"time"

	"reasonix/internal/sessioninbox"
)

// The chip reads these columns through the bridge, so the wire keys are the
// contract: a rename must fail here rather than silently drop the column. Absent
// stays absent too — a queued item nobody refused carries no refusal.
func TestInboxViewWireColumnsForAWaitingLine(t *testing.T) {
	snap := sessioninbox.InboxSnapshot{Items: []sessioninbox.InboxItemMeta{
		{
			ID: "it-waiting", State: sessioninbox.StateQueued, Source: "push", Preview: "点名",
			CreatedAt: time.Now().Add(-2 * time.Second),
			Room:      &sessioninbox.RoomMeta{Seq: 43, From: "chatside", Topic: 6},
		},
		{ID: "it-plain", State: sessioninbox.StateQueued, Source: "desktop", Preview: "无房"},
	}}
	view := inboxSnapshotView(snap, func(id string) (string, string, bool, bool) {
		if id != "it-waiting" {
			return "", "", false, false
		}
		return "dispatch", "这个会话在桌面端已经没有标签页在托管它", false, true
	})
	if len(view.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(view.Items))
	}

	raw, err := json.Marshal(view.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"waitMs", "waitGate", "waitReason", "waitRefused", "room"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("waiting item is missing %q: %s", key, raw)
		}
	}
	if fields["waitGate"] != "dispatch" || fields["waitReason"] != "这个会话在桌面端已经没有标签页在托管它" {
		t.Fatalf("the host's own words did not survive: %s", raw)
	}
	if resumable, ok := fields["waitResumable"]; ok {
		t.Fatalf("resumable showed up although nobody said another wake could lift it: %v", resumable)
	}
	room, _ := fields["room"].(map[string]any)
	if room == nil || room["seq"] != float64(43) || room["from"] != "chatside" {
		t.Fatalf("room meta did not survive: %s", raw)
	}

	plain, err := json.Marshal(view.Items[1])
	if err != nil {
		t.Fatal(err)
	}
	plainFields := map[string]any{}
	if err := json.Unmarshal(plain, &plainFields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"waitMs", "waitGate", "waitReason", "waitRefused", "room"} {
		if _, ok := plainFields[key]; ok {
			t.Fatalf("an item nobody is waiting on carries %q: %s", key, plain)
		}
	}
}
