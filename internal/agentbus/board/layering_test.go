package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The board is a tool layer: it decides transitions, not schedules. Reaching
// either the agent or the controller package would put orchestration or
// transport behind a data structure the whole cluster writes to.
func TestPackageStaysBelowAgentAndControl(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no package files found; run this test from the package directory")
	}
	forbidden := []string{
		"reasonix/internal/control",
		"reasonix/internal/agent\"",
		"reasonix/internal/agent/",
		"reasonix/internal/serve",
		"reasonix/internal/boot",
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, dep := range forbidden {
			if strings.Contains(string(body), dep) {
				t.Fatalf("%s imports %s; the board must stay below it", name, dep)
			}
		}
	}
}
