package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/event"
)

// B4 (docs/50 §2.3): every site that folds text into the outgoing turn declares
// its delivery class. The map is the declaration — a new or reworded injection
// fails this guard rather than quietly joining a line it does not belong to.
//
// Class A (external guidance) may appear here exactly once, because the turn body
// is where external guidance legitimately lands; the inbox wake path is the other
// entry point and lives outside compose. Classes B (host notices) and C
// (projections) must never reach the body at all.
var turnTailSites = map[string]event.DeliveryClass{
	`text = prefix + "\n\n" + text`:                                                event.DeliverySessionState,
	`text = PlanModeMarker + "\n\n" + text`:                                        event.DeliverySessionState,
	`text = agent.WithResponseLanguage(text, responseLanguage)`:                    event.DeliverySessionState,
	`text = agent.WithReasoningLanguageForSource(text, reasoningLanguage, source)`: event.DeliverySessionState,
	`text = b.String() + text`:                                                     event.DeliverySessionState,
	`text = progress + "\n\n" + text`:                                              event.DeliverySessionState,
	`text = "<background-jobs>\n" + note + "\n</background-jobs>\n\n" + text`:      event.DeliverySessionState,
	`text = "<agentbus-view>\n" + block + "</agentbus-view>\n\n" + text`:           event.DeliverySessionState,
	`text = "<agentbus-talk>\n" + block + "</agentbus-talk>\n\n" + text`:           event.DeliverySessionState,
	`text = block + "\n\n" + text`:                                                 event.DeliveryExternalGuidance,
	`text = strings.TrimRight(text, "\n") + "\n\n" + block`:                        event.DeliverySessionState,
	`text = c.offerProjectOffers(text, source)`:                                    event.DeliverySessionState,
}

func composeWithGoalBody(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("input.go")
	if err != nil {
		t.Fatalf("read input.go: %v", err)
	}
	source := string(data)
	start := strings.Index(source, "func (c *Controller) composeWithGoal(")
	if start < 0 {
		t.Fatal("composeWithGoal not found")
	}
	end := strings.Index(source[start+10:], "\nfunc ")
	if end < 0 {
		t.Fatal("composeWithGoal body has no following declaration")
	}
	return source[start : start+10+end]
}

func TestTurnTailInjectionsDeclareTheirClass(t *testing.T) {
	body := composeWithGoalBody(t)
	declared := map[string]int{}
	for raw := range strings.SplitSeq(body, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "text =") {
			continue
		}
		class, ok := turnTailSites[line]
		if !ok {
			t.Fatalf("unclassified turn-tail injection: %q — decide its delivery class (docs/50 §2.3)", line)
		}
		declared[line]++
		switch class {
		case event.DeliveryHostNotice, event.DeliveryProjection:
			t.Fatalf("%q is declared %s, which must never enter the turn body", line, class)
		case event.DeliveryExternalGuidance:
			if declared[line] > 1 {
				t.Fatalf("%q declared twice", line)
			}
		}
	}
	for line := range turnTailSites {
		if declared[line] == 0 {
			t.Fatalf("turnTailSites declares %q, which no longer appears in composeWithGoal", line)
		}
	}
	guidance := 0
	for _, class := range turnTailSites {
		if class == event.DeliveryExternalGuidance {
			guidance++
		}
	}
	if guidance != 1 {
		t.Fatalf("external guidance sites = %d, want exactly the hook context block", guidance)
	}
	if strings.Contains(body, "KindNotice") || strings.Contains(body, "event.Notice") {
		t.Fatal("composeWithGoal must not fold a host notice into the turn body")
	}
}

// The wake marker is the other class-A entry point, and it has exactly one
// producer: the inbox path. Nothing else spells the marker.
func TestRemoteWakeMarkerHasOneProducer(t *testing.T) {
	root := filepath.Join("..")
	var producers []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), `"[remote wake source="`) {
			producers = append(producers, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	if len(producers) != 1 || filepath.Base(producers[0]) != "inbox_wake_marker.go" || !strings.Contains(producers[0], "/control/") {
		t.Fatalf("marker producers = %v, want exactly control/inbox_wake_marker.go", producers)
	}
}
