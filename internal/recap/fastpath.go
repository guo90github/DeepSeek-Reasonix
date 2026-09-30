package recap

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// unprovenFastReads remembers transcript versions the sidecar digest did not
// prove, keyed by path plus size and mtime. Closing the same session again then
// goes straight to the replay instead of paying for the parse a second time.
var unprovenFastReads sync.Map

func fastReadKey(path string, info os.FileInfo) string {
	return fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
}

func fastReadUnproven(key string) bool {
	_, unproven := unprovenFastReads.Load(key)
	return unproven
}

func markFastReadUnproven(key string) {
	unprovenFastReads.Store(key, struct{}{})
}

// verifyFastPath compares a fast transcript read against the authoritative
// render for the first few attempts of a process, so a deployed build proves its
// own fast path instead of trusting a corpus run. A disagreement is recorded and
// the authoritative text wins; the budget keeps every later close free of the
// replay this feature exists to avoid.
func verifyFastPath(ctx context.Context, g *Generator, store *Store, path, text string) string {
	if g == nil {
		return text
	}
	select {
	case <-g.shadowBudget:
	default:
		return text
	}
	shadow, ok := g.opts.Transcript.(ShadowTranscript)
	if !ok {
		return text
	}
	authoritative, err := shadow.ReadAuthoritative(ctx, path)
	if err != nil {
		g.trace(ctx, store, "fastpath", path, "authoritative read failed")
		return text
	}
	if authoritative == text {
		g.trace(ctx, store, "fastpath", path, "verified")
		return text
	}
	g.trace(ctx, store, "fastpath", path, "mismatch: took the replay")
	return authoritative
}
