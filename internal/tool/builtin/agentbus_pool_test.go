package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// poolFake is the fake board with a pool: the pool is a read the tool has to be able to make.
type poolFake struct {
	*fakeBoardPort
	pool []agentbus.PoolEntry
}

func (f poolFake) BoardPool() ([]agentbus.PoolEntry, error) { return f.pool, nil }

// Nothing broadcasts the pool, so a session that wants to help has to be able to ask for it
// (F53/F40, 2026-10-05).
func TestAgentBusToolListsThePool(t *testing.T) {
	port := poolFake{
		fakeBoardPort: &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"},
		pool:          []agentbus.PoolEntry{{ID: "free", Title: "nobody wants it", LastSeq: 3}},
	}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"pool"}`))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	for _, want := range []string{"1 step(s) in the pool", "free", `title="nobody wants it"`, "waiters=0", "last_seq=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the pool does not carry %q:\n%s", want, out)
		}
	}
}

// An empty pool says so rather than showing nothing: "nothing to take" and "nobody moved
// anything yet" are different facts (2026-10-05).
func TestAgentBusToolSaysWhenThePoolIsEmpty(t *testing.T) {
	port := poolFake{fakeBoardPort: &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"pool"}`))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if !strings.Contains(out, "the pool is empty") {
		t.Fatalf("the empty pool reads %q", out)
	}
}
