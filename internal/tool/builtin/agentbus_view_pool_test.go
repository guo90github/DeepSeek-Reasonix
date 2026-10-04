package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// "There is work to take" has to be discoverable from the read a session already makes: the pool
// is a pull path, and a pull path nobody knows about is no path at all (F53/F40, 2026-10-05).
func TestTheViewPointsAtThePoolWhenSomethingIsInIt(t *testing.T) {
	port := poolFake{
		fakeBoardPort: &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"},
		pool:          []agentbus.PoolEntry{{ID: "free", Title: "nobody wants it", LastSeq: 3}},
	}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"view"}`))
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if !strings.Contains(out, "1 step(s) in the pool") || !strings.Contains(out, "action=pool") {
		t.Fatalf("the view does not point at the pool:\n%s", out)
	}

	// An empty pool is not worth a line: a pointer to nothing is noise.
	empty := poolFake{fakeBoardPort: &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}}
	out, err = NewAgentBusTool(empty).Execute(context.Background(), boardArgs(t, `{"action":"view"}`))
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if strings.Contains(out, "action=pool") {
		t.Fatalf("an empty pool was pointed at:\n%s", out)
	}
}
