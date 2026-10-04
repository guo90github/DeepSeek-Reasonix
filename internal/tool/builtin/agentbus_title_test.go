package builtin

import (
	"context"
	"strings"
	"testing"
)

// A node an op brings into being has to be named in that same op: the title is how a later
// reader finds it, and no verb in this vocabulary renames a node, so a titleless child can
// only ever be found by its id (2026-10-05).
func TestAgentBusToolNamesTheNodesItCreates(t *testing.T) {
	cases := []struct{ name, args string }{
		{"a split child", `{"action":"split","node":"root","children":[{"id":"a","title":"first"},{"id":"b"}]}`},
		{"a required dependency", `{"action":"require","node":"build","dep":{"id":"key"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port := &fakeBoardPort{dir: "/tmp/board/default", participant: "alice"}
			_, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, c.args))
			if err == nil {
				t.Fatalf("%s reached the board unnamed", c.name)
			}
			if !strings.Contains(err.Error(), "title") {
				t.Fatalf("error = %q, want it to name the missing title", err)
			}
			if len(port.applied) != 0 {
				t.Fatalf("a refused call must not reach the board: %+v", port.applied)
			}
		})
	}
}
