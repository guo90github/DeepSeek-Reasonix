package agent

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSessionDigestSeededFromIntactEventIndex pins that the sidecar-index
// shortcut yields the byte-identical digest full hashing produces, and that
// the shortcut actually runs for an intact index.
func TestLoadSessionDigestSeededFromIntactEventIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	s := NewSession("sys")
	for _, m := range representativeSessionMessages()[1:] {
		s.Add(m)
	}
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if _, err := os.Stat(SessionEventIndexPath(path)); err != nil {
		t.Fatalf("expected event index sidecar: %v", err)
	}

	before := sessionEventDigestSeeds.Load()
	msgs, fromEvents, damaged, digest, digestOK, err := loadAndDigestSessionMessages(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !fromEvents || damaged {
		t.Fatalf("fromEvents=%v damaged=%v, want clean event-log replay", fromEvents, damaged)
	}
	if !digestOK {
		t.Fatal("digestOK = false, want true")
	}
	want, err := legacyDigestSessionMessages(msgs)
	if err != nil {
		t.Fatalf("legacy digest: %v", err)
	}
	if digest != want {
		t.Fatalf("seeded digest %x != full-hash digest %x", digest, want)
	}
	if sessionEventDigestSeeds.Load() == before {
		t.Fatal("intact index did not engage the digest shortcut")
	}
}

// TestLoadSessionDigestFallsBackOnStaleIndex verifies a LogSize mismatch in
// the sidecar disarms the shortcut, loads normally, and still matches the
// full-hash digest.
func TestLoadSessionDigestFallsBackOnStaleIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	s := NewSession("sys")
	for _, m := range representativeSessionMessages()[1:] {
		s.Add(m)
	}
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	idxPath := SessionEventIndexPath(path)
	b, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx sessionEventIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatalf("decode index: %v", err)
	}
	idx.LogSize += 1000
	stale, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("encode index: %v", err)
	}
	if err := os.WriteFile(idxPath, stale, 0o600); err != nil {
		t.Fatalf("write stale index: %v", err)
	}

	before := sessionEventDigestSeeds.Load()
	msgs, fromEvents, damaged, digest, digestOK, err := loadAndDigestSessionMessages(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !fromEvents || damaged {
		t.Fatalf("fromEvents=%v damaged=%v, want clean event-log replay", fromEvents, damaged)
	}
	if !digestOK {
		t.Fatal("digestOK = false, want true")
	}
	want, err := legacyDigestSessionMessages(msgs)
	if err != nil {
		t.Fatalf("legacy digest: %v", err)
	}
	if digest != want {
		t.Fatalf("fallback digest %x != full-hash digest %x", digest, want)
	}
	if sessionEventDigestSeeds.Load() != before {
		t.Fatal("stale index still engaged the digest shortcut")
	}
}

// TestLoadSessionDigestSeedNilHasher verifies the nil-hasher contract on both
// the load path and the seed method itself.
func TestLoadSessionDigestSeedNilHasher(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	s := NewSession("sys")
	for _, m := range representativeSessionMessages()[1:] {
		s.Add(m)
	}
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	msgs, fromEvents, damaged, err := loadSessionMessages(path)
	if err != nil {
		t.Fatalf("load with nil hasher: %v", err)
	}
	if !fromEvents || damaged {
		t.Fatalf("fromEvents=%v damaged=%v, want clean event-log load", fromEvents, damaged)
	}
	if len(msgs) == 0 {
		t.Fatal("loaded empty transcript")
	}

	var seed [sha256.Size]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	var nilHasher *sessionTranscriptHasher
	nilHasher.seedDigest(seed) // must not panic
	if _, ok := nilHasher.sum(); ok {
		t.Fatal("nil hasher sum reported ok after seed")
	}
	hasher := newSessionTranscriptHasher()
	hasher.seedDigest(seed)
	got, ok := hasher.sum()
	if !ok {
		t.Fatal("sum ok = false after seed")
	}
	if got != seed {
		t.Fatalf("seeded sum %x != seed %x", got, seed)
	}
}
