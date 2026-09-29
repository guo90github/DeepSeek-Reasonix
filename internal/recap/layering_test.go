package recap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The recap lane must never reach for the controller: the kernel cannot depend
// on a frontend-owned type, and every trigger lives one layer up.
func TestPackageDoesNotReachController(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no package files found; run this test from the package directory")
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "reasonix/internal/control") {
			t.Fatalf("%s imports reasonix/internal/control; the recap lane must stay below it", name)
		}
	}
}
