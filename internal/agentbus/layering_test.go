package agentbus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The machine channel is a tool layer: it projects the blackboard and never
// reaches up into the controller, a scheduler or a frontend. Reaching up would
// make the view's size a policy of whoever wired it.
func TestPackageStaysBelowControl(t *testing.T) {
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
				t.Fatalf("%s imports %s; agentbus must stay below it", name, dep)
			}
		}
	}
}
