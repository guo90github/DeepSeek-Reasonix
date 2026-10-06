package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestArchiveStemBoundsLongNames: the archive component adds a 20-byte
// timestamp to a name slug had already bounded to 252, so a long fact's forget
// used to fail with ENAMETOOLONG instead of archiving.
func TestArchiveStemBoundsLongNames(t *testing.T) {
	when := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if got := archiveStem("short-fact", when); got != "20261006-120000.000-short-fact" {
		t.Fatalf("a short name must pass through unchanged, got %q", got)
	}
	long := archiveStem(strings.Repeat("x", 400), when)
	if !strings.HasPrefix(long, "20261006-120000.000-") {
		t.Fatalf("timestamp prefix lost, so archiveTimeFromName breaks: %q", long)
	}
	if len(long)+len(".md") > 255 {
		t.Fatalf("archived component is %d bytes, over the 255-byte limit", len(long)+len(".md"))
	}
}

func TestArchiveHandlesLongFactNames(t *testing.T) {
	store := recallTestStore(t)
	long := strings.Repeat("always-verify-the-desktop-session-lease-before-rebuild-", 8) + "end"
	saved, err := store.SaveWithOptions(Memory{Name: long, Description: "d", Body: "b"}, SaveOptions{})
	if err != nil {
		t.Fatalf("Save long name: %v", err)
	}
	archive, err := store.Archive(saved.Memory.Name)
	if err != nil {
		t.Fatalf("Archive of a long-named fact failed: %v", err)
	}
	if archive == "" {
		t.Fatal("nothing was archived")
	}
	if base := filepath.Base(archive); len(base) > 255 {
		t.Fatalf("archived component is %d bytes: %s", len(base), base)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archived file is missing: %v", err)
	}
	if _, ok := store.Read(saved.Memory.Name); ok {
		t.Fatal("the fact is still active after archiving")
	}
}

func TestFactCapMessageNamesOldestFacts(t *testing.T) {
	store := recallTestStore(t)
	for i := range MaxProjectFacts {
		if _, err := store.Save(Memory{Name: fmt.Sprintf("fact-%02d", i), Body: "body"}); err != nil {
			t.Fatalf("fact %d rejected: %v", i, err)
		}
	}
	_, err := store.Save(Memory{Name: "one-too-many", Body: "body"})
	if err == nil {
		t.Fatal("the cap must reject the extra fact")
	}
	if !strings.Contains(err.Error(), "fact-00") {
		t.Fatalf("the rejection must name what to curate, got %v", err)
	}
}

func TestAllowOverCapLetsUserConfirmedWriteThrough(t *testing.T) {
	store := recallTestStore(t)
	for i := range MaxProjectFacts {
		if _, err := store.Save(Memory{Name: fmt.Sprintf("cap-%02d", i), Body: "body"}); err != nil {
			t.Fatalf("fact %d rejected: %v", i, err)
		}
	}
	if _, err := store.SaveWithOptions(Memory{Name: "user-confirmed", Body: "body"}, SaveOptions{}); err == nil {
		t.Fatal("a model-driven write must still be capped")
	}
	if _, err := store.SaveWithOptions(Memory{Name: "user-confirmed", Body: "body"}, SaveOptions{AllowOverCap: true}); err != nil {
		t.Fatalf("an explicit user-confirmed write must pass the cap: %v", err)
	}
}
