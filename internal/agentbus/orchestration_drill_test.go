package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// The drill `docs/agents/ORCHESTRATION.md` §5 promises: the planner writes the deliverable and
// one block, then disappears. A survivor reads only the files and — the kernel has no automatic
// planner — grows the graph itself, following the documented rules: the deliverable is the root
// it waits for its blocks, and a block nothing waits on is the survivor's to add instead of
// "picking up the parked step" (the half takeover_test.go already covers).
func TestASurvivorGrowsTheNodeGraphAfterTheOrchestratorIsGone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	planner, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// §1: the deliverable first, then the block it waits for (require: node waits for dep).
	if _, err := planner.ApplyAll(ctx,
		board.Op{Verb: board.VerbAssert, Node: "release", Actor: "planner", Evidence: e2eEvidence()},
		board.Op{Verb: board.VerbRequire, Node: "release", Actor: "planner",
			Dep: &board.NodeSpec{ID: "build", Title: "Build the release"}},
	); err != nil {
		t.Fatalf("planner lays out the graph: %v", err)
	}
	// The planner is gone: nothing is ever written in its name again. (Its process, handles
	// and memory are what takeover_test.go drops; this is the growth half.)

	survivor, err := board.Open(dir)
	if err != nil {
		t.Fatalf("survivor opens: %v", err)
	}
	claim := func(node string) {
		t.Helper()
		if _, err := survivor.Apply(ctx, board.Op{
			Verb: board.VerbClaim, Node: node, Actor: "survivor",
			Bounds:   &board.Bounds{Steps: 2},
			Deadline: time.Now().UTC().Add(time.Hour),
		}); err != nil {
			t.Fatalf("claim %q: %v", node, err)
		}
	}
	// decide(done) needs the node's own evidence and a reproducer who did not produce it.
	finish := func(node, kind, ref string) {
		t.Helper()
		if _, err := survivor.Apply(ctx, board.Op{
			Verb: board.VerbAssert, Node: node, Actor: "survivor",
			Evidence: []board.Evidence{{Kind: kind, Ref: ref}},
		}); err != nil {
			t.Fatalf("assert %q: %v", node, err)
		}
		if _, err := survivor.Apply(ctx, board.Op{
			Verb: board.VerbDecide, Node: node, Actor: "survivor", Outcome: board.OutcomeDone,
			ReproducedBy: "reviewer",
		}); err != nil {
			t.Fatalf("decide %q: %v", node, err)
		}
	}

	claim("build")
	finish("build", "test", "go test ./internal/agentbus/...")

	// Reading the board back is what shows the graph is short: the deliverable still waits for
	// nothing that exists. Landing it now would be landing an empty promise.
	state := e2eSnapshot(t, dir)
	if landing := AssessLanding(state, NewHearingState()); landing.Landed {
		t.Fatalf("landed with the deliverable never done: %+v", landing)
	}

	// So the survivor grows the graph the same way the planner did.
	if _, err := survivor.Apply(ctx, board.Op{
		Verb: board.VerbRequire, Node: "release", Actor: "survivor",
		Dep: &board.NodeSpec{ID: "package", Title: "Pack the release"},
	}); err != nil {
		t.Fatalf("grow the graph: %v", err)
	}
	claim("package")
	finish("package", "command", "scripts/desktop-build.sh windows/amd64 v0.0.0-dev.1")

	claim("release")
	finish("release", "file", "dist/Reasonix-windows-amd64.zip")

	state = e2eSnapshot(t, dir)
	if landing := AssessLanding(state, NewHearingState()); !landing.Landed {
		t.Fatalf("landing = %+v, want the grown graph to converge", landing)
	}
	// The structural nodes carry the titles the spec requires of require/split: a node the
	// panel can only show as an id is a node a person cannot plan from (§13.11's boundary).
	for id, want := range map[string]string{"build": "Build the release", "package": "Pack the release"} {
		if n := state.Nodes[id]; n == nil || n.Title != want {
			t.Fatalf("node %q = %+v, want title %q", id, n, want)
		}
	}

	// The ending lives in the files, not in the survivor's memory: a reader that was never part
	// of any of this folds the same one.
	fresh, err := board.Open(dir)
	if err != nil {
		t.Fatalf("fresh reader opens: %v", err)
	}
	again, err := fresh.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("fresh reader folds: %v", err)
	}
	if !AssessLanding(again, NewHearingState()).Landed {
		t.Fatal("a reader that never took part must reach the same ending")
	}
}
