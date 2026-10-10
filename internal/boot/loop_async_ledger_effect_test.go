package boot

// The ledger for a lifted call, at the real boundary: moving the slow command
// off the critical path buys wall clock only when the turn has something else to
// do, and it always costs the round the deferred call has to be sent again.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

const (
	ledgerSlowArgs  = `{"command":"sleep 2"}`
	ledgerCheckArgs = `{"command":"make test"}`
	// ledgerMakefile makes the check slow for real: `make test` is what the host
	// recognizes as a check, and the recipe is what costs the two seconds.
	ledgerMakefile = "test:\n\tsleep 2\n"
)

// ledgerProvider plays one shape of turn: it asks for a slow command and an
// independent read, then reacts to what the host did with them. It never learns
// the configured tier — the tool results tell it whether the read ran.
type ledgerProvider struct {
	// collects is the shape where the turn needs the slow output before it ends.
	collects bool
	slowArgs string

	mu        sync.Mutex
	round     int
	resent    bool
	collected bool
	reqs      []provider.Request
}

func (*ledgerProvider) Name() string { return "boot-loop-ledger" }

func (p *ledgerProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.round++
	round := p.round
	p.reqs = append(p.reqs, req)
	script := p.script(round, req)
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 4)
	if len(script) == 0 {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	for _, call := range script {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: call}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// script is the provider's own ledger read back: a deferred read is sent again,
// and a turn that needs the slow output waits for the job it started.
func (p *ledgerProvider) script(round int, req provider.Request) []*provider.ToolCall {
	switch {
	case round == 1:
		return []*provider.ToolCall{
			{ID: "slow", Name: "bash", Arguments: p.slowArgs},
			{ID: "read1", Name: "read_file", Arguments: `{"path":"note.txt"}`},
		}
	case !p.resent && deferredIn(req):
		p.resent = true
		return []*provider.ToolCall{{ID: "read2", Name: "read_file", Arguments: `{"path":"note.txt"}`}}
	case p.collects && !p.collected:
		p.collected = true
		return []*provider.ToolCall{{ID: "wait1", Name: "wait", Arguments: `{}`}}
	}
	return nil
}

func (p *ledgerProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// deferredIn reports whether the host settled a batch with a call left behind,
// which is what a model learns from the "collect it first" tool result.
func deferredIn(req provider.Request) bool {
	for _, msg := range req.Messages {
		if msg.Role == provider.RoleTool && strings.Contains(msg.Content, "moved to the background") {
			return true
		}
	}
	return false
}

// measureLedger runs one turn of this shape and reports what it cost.
func measureLedger(t *testing.T, collects bool, tier, slowArgs, kind string) (time.Duration, int) {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	if slowArgs == ledgerCheckArgs {
		writeFile(t, dir, "Makefile", ledgerMakefile)
	}
	rec := &ledgerProvider{collects: collects, slowArgs: slowArgs}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "note.txt", "alpha\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
shell_async = "`+tier+`"

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	start := time.Now()
	if err := ctrl.Run(context.Background(), "run the slow thing and read the note"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return time.Since(start), len(rec.requests())
}

// TestEffectOffPathLedgerForALiftedCall measures both sides of the tier:
// wall clock and provider rounds for the same batch under off and under fast.
func TestEffectOffPathLedgerForALiftedCall(t *testing.T) {
	for _, tc := range []struct {
		name     string
		collects bool
	}{
		{name: "the turn has independent work while the slow call runs"},
		{name: "the turn needs the slow output before it ends", collects: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shape := map[bool]string{true: "collects", false: "independent"}[tc.collects]
			var offWall, fastWall time.Duration
			var offRounds, fastRounds int
			t.Run("off", func(t *testing.T) {
				offWall, offRounds = measureLedger(t, tc.collects, "off", ledgerSlowArgs, "boot-ledger-off-"+shape)
			})
			t.Run("fast", func(t *testing.T) {
				fastWall, fastRounds = measureLedger(t, tc.collects, "fast", ledgerSlowArgs, "boot-ledger-fast-"+shape)
			})

			t.Logf("MEASURED: collects=%v off: wall=%.0fms rounds=%d | fast: wall=%.0fms rounds=%d",
				tc.collects, offWall.Seconds()*1000, offRounds, fastWall.Seconds()*1000, fastRounds)

			if !tc.collects {
				if fastWall >= offWall-time.Second {
					t.Fatalf("the lifted call did not leave the critical path: off=%.0fms fast=%.0fms",
						offWall.Seconds()*1000, fastWall.Seconds()*1000)
				}
				if fastRounds != offRounds+1 {
					t.Fatalf("the deferral did not cost exactly one round: off=%d fast=%d", offRounds, fastRounds)
				}
				return
			}
			// The shape that needs the output buys no wall clock, so the extra
			// rounds it pays for are a net loss.
			if fastWall < offWall-time.Second {
				t.Fatalf("a turn that waits for the job cannot be faster: off=%.0fms fast=%.0fms",
					offWall.Seconds()*1000, fastWall.Seconds()*1000)
			}
			if fastRounds <= offRounds {
				t.Fatalf("collecting the job must cost rounds: off=%d fast=%d", offRounds, fastRounds)
			}
		})
	}
}

// TestEffectOffPathLedgerForARecognizedCheck answers what the middle tier buys.
// `make test` is a check the host recognizes, which is the only thing balanced
// gates on — off never lifts and fast ignores the classification, so this test
// also cross-checks that the check behaves like any other command on those two.
func TestEffectOffPathLedgerForARecognizedCheck(t *testing.T) {
	for _, tc := range []struct {
		name     string
		collects bool
	}{
		{name: "the turn has independent work while the check runs"},
		{name: "the turn needs the check result before it ends", collects: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shape := map[bool]string{true: "collects", false: "independent"}[tc.collects]
			wall, rounds := map[string]time.Duration{}, map[string]int{}
			for _, tier := range []string{"off", "balanced", "fast"} {
				t.Run(tier, func(t *testing.T) {
					wall[tier], rounds[tier] = measureLedger(t, tc.collects, tier, ledgerCheckArgs, "boot-check-"+tier+"-"+shape)
				})
			}

			t.Logf("MEASURED: check=%q collects=%v off: wall=%.0fms rounds=%d | balanced: wall=%.0fms rounds=%d | fast: wall=%.0fms rounds=%d",
				ledgerCheckArgs, tc.collects,
				wall["off"].Seconds()*1000, rounds["off"],
				wall["balanced"].Seconds()*1000, rounds["balanced"],
				wall["fast"].Seconds()*1000, rounds["fast"])

			if wall["off"] < 1800*time.Millisecond {
				t.Fatalf("the check did not run to completion under off (%.0fms)", wall["off"].Seconds()*1000)
			}
			if rounds["balanced"] != rounds["fast"] {
				t.Fatalf("a recognized check must behave the same on both lifting tiers: balanced=%d fast=%d",
					rounds["balanced"], rounds["fast"])
			}
			if !tc.collects {
				if wall["balanced"] >= wall["off"]-time.Second {
					t.Fatalf("the lifted check did not leave the critical path: off=%.0fms balanced=%.0fms",
						wall["off"].Seconds()*1000, wall["balanced"].Seconds()*1000)
				}
				if rounds["balanced"] != rounds["off"]+1 {
					t.Fatalf("the deferral did not cost exactly one round: off=%d balanced=%d", rounds["off"], rounds["balanced"])
				}
				return
			}
			if wall["balanced"] < wall["off"]-time.Second {
				t.Fatalf("a turn that waits for the check cannot be faster: off=%.0fms balanced=%.0fms",
					wall["off"].Seconds()*1000, wall["balanced"].Seconds()*1000)
			}
			if rounds["balanced"] <= rounds["off"] {
				t.Fatalf("collecting the check must cost rounds: off=%d balanced=%d", rounds["off"], rounds["balanced"])
			}
		})
	}
}
