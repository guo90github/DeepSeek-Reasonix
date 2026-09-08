package agent

import (
	"crypto/sha256"
	"path/filepath"
	"testing"
)

// TestLoadSessionDigestSeedNilHasher verifies the nil-hasher contract on both
// the load path and the seed method itself (dev-2 legacy, kept for coverage).
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
