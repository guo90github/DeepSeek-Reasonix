package main

import (
	"testing"

	"reasonix/internal/sessioninbox"
)

// The bridge-facing snapshot keeps the chat-room origin so the frontend can
// badge a wake without re-deriving it from the preview text.
func TestInboxSnapshotViewCarriesRoom(t *testing.T) {
	snap := sessioninbox.InboxSnapshot{
		Items: []sessioninbox.InboxItemMeta{{
			ID:      "i1",
			Preview: "Chat room #43: fusion-root mentioned you",
			Source:  "push",
			Room: &sessioninbox.RoomMeta{
				Seq: 43, From: "fusion-root", Topic: 4, Kind: "say", Origin: "agent",
				Mentions: []string{"reasonix-host"},
			},
		}},
	}
	view := inboxSnapshotView(snap)
	if len(view.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(view.Items))
	}
	room := view.Items[0].Room
	if room == nil {
		t.Fatal("room = nil, want the snapshot's room meta")
	}
	if room.Seq != 43 || room.From != "fusion-root" || room.Topic != 4 || room.Kind != "say" || room.Origin != "agent" {
		t.Fatalf("room = %+v, want seq=43 from=fusion-root topic=4 kind=say origin=agent", room)
	}
	if len(room.Mentions) != 1 || room.Mentions[0] != "reasonix-host" {
		t.Fatalf("mentions = %v, want [reasonix-host]", room.Mentions)
	}
}

func TestInboxSnapshotViewLeavesRoomNilForPlainItems(t *testing.T) {
	snap := sessioninbox.InboxSnapshot{
		Items: []sessioninbox.InboxItemMeta{{ID: "i1", Preview: "plain push", Source: "push"}},
	}
	view := inboxSnapshotView(snap)
	if len(view.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(view.Items))
	}
	if view.Items[0].Room != nil {
		t.Fatalf("room = %+v, want nil", view.Items[0].Room)
	}
}
