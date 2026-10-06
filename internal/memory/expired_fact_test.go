package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestIndexHidesExpiredFacts(t *testing.T) {
	store := recallTestStore(t)
	live, err := store.SaveWithOptions(Memory{Name: "live-fact", Description: "still applies", Body: "body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spent, err := store.SaveWithOptions(Memory{
		Name: "expired-fact", Description: "past its expiry", Body: "body",
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}

	index := store.Index()
	if !strings.Contains(index, live.Memory.ID) {
		t.Fatalf("live fact missing from the index: %q", index)
	}
	if strings.Contains(index, spent.Memory.ID) {
		t.Fatalf("expired fact still rides the index: %q", index)
	}
}

func TestFactCapIgnoresExpiredFacts(t *testing.T) {
	store := recallTestStore(t)
	for i := range MaxProjectFacts {
		if _, err := store.Save(Memory{
			Name:      "spent-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Body:      "body",
			ExpiresAt: time.Now().UTC().Add(-time.Hour),
		}); err != nil {
			t.Fatalf("expired fact %d was rejected: %v", i, err)
		}
	}
	if _, err := store.Save(Memory{Name: "new-live-fact", Description: "fresh", Body: "body"}); err != nil {
		t.Fatalf("a live fact must fit while expired facts hold no slot: %v", err)
	}
}

func TestForgetReportsMissedNameWithoutClaimingSuccess(t *testing.T) {
	store := recallTestStore(t)
	store.Save(Memory{Name: "kept-fact", Description: "kept", Body: "body"})

	out, err := NewForgetTool(store).Execute(context.Background(), []byte(`{"name":"no-such-fact"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No active memory named") {
		t.Fatalf("a missed name must say nothing was archived, got %q", out)
	}
	if strings.Contains(out, "will not load in future sessions") {
		t.Fatalf("a missed name must not read as a completed delete, got %q", out)
	}
	if _, ok := store.Read("kept-fact"); !ok {
		t.Fatal("an unrelated fact was disturbed")
	}
}

func TestForgetSuggestsNearMissName(t *testing.T) {
	store := recallTestStore(t)
	store.Save(Memory{Name: "desktop-split-pane-fix", Description: "real fact", Body: "body"})

	out, err := NewForgetTool(store).Execute(context.Background(), []byte(`{"name":"desktop-split-pane-fx"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "desktop-split-pane-fix") {
		t.Fatalf("a one-character miss must point at the real name, got %q", out)
	}
	if _, ok := store.Read("desktop-split-pane-fix"); !ok {
		t.Fatal("the suggested fact itself was disturbed")
	}
}
