package recap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/store"
)

// PromptVersion tags the prompt that produced a record so a prompt change can
// invalidate stored records without touching session content.
const PromptVersion = "recap-v8"

// ErrNoTranscript reports that a session has no authoritative file to read.
var ErrNoTranscript = fmt.Errorf("recap: session has no transcript")

// Record is one session recap plus the provenance that decides whether it is
// still current. Fingerprint identifies the session content it was made from.
type Record struct {
	Path          string    `json:"path"`
	Fingerprint   string    `json:"fingerprint"`
	Entries       []Entry   `json:"entries"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"promptVersion"`
	GeneratedAt   time.Time `json:"generatedAt"`
}

// Fingerprint identifies a session's authoritative content by the sizes and
// modification times of its transcript and event log. It is cheap enough to
// recompute on every close, unlike hashing a transcript that may be megabytes.
func Fingerprint(sessionPath string) (string, error) {
	paths := authoritativeFiles(sessionPath)
	if len(paths) == 0 {
		return "", ErrNoTranscript
	}
	h := sha256.New()
	found := false
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		found = true
		fmt.Fprintf(h, "%s|%d|%d\n", filepath.Base(path), info.Size(), info.ModTime().UnixNano())
	}
	if !found {
		return "", ErrNoTranscript
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// authoritativeFiles lists the files whose content a recap must reflect: the
// compatibility transcript and, when present, the native event log that holds
// the authoritative copy of the same conversation.
func authoritativeFiles(sessionPath string) []string {
	path := strings.TrimSpace(sessionPath)
	if path == "" {
		return nil
	}
	out := []string{path}
	if log := strings.TrimSpace(store.SessionEventLog(path)); log != "" {
		out = append(out, log)
	}
	return out
}
