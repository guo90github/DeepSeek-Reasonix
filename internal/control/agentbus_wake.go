package control

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// SetAgentBusWaker installs the host's routing for ready-work wakes. The kernel
// decides who has work waiting and why; only the host knows how to reach them — a
// sibling controller's inbox in this process, a tab, or another machine. Without a
// waker the kernel wakes nobody, which is the safe default: the host's own tick
// scan stays the fallback (AGENT_BUS §13.2).
func (c *Controller) SetAgentBusWaker(fn func(context.Context, agentbus.WakeTarget) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.waker = fn
	}
}

// WakeFailures is the host's record of wakes it could not hand over: which participant and
// why, for the last one. A wake that reaches nobody leaves the board standing still and the
// sender's log as its only witness — the same invisible brake a spent ceiling is (G3).
type WakeFailures struct {
	Count int64
	Last  string
}

// wakeFailures is process-wide like the refusal counts: the routing belongs to the machine.
var wakeFailures = struct {
	mu    sync.Mutex
	count atomic.Int64
	last  string
}{}

// AgentBusWakeFailures reports the wakes this process could not deliver.
func AgentBusWakeFailures() WakeFailures {
	wakeFailures.mu.Lock()
	defer wakeFailures.mu.Unlock()
	return WakeFailures{Count: wakeFailures.count.Load(), Last: wakeFailures.last}
}

func noteWakeFailure(participant string, err error) {
	detail := participant + ": " + err.Error()
	if len(detail) > maxWakeFailureDetail {
		detail = detail[:maxWakeFailureDetail] + "…"
	}
	wakeFailures.mu.Lock()
	defer wakeFailures.mu.Unlock()
	wakeFailures.count.Add(1)
	wakeFailures.last = detail
}

// A refusal's message is the host's own, but a delivery error can wrap a peer's whole body,
// and this row is read in a panel line rather than a log.
const maxWakeFailureDetail = 160

// wakeUnreachable remembers the participants this process already found no route to, so each one is
// logged once instead of every 30 seconds. It is deliberately NOT what the panel reports: a board
// outlives the sessions that wrote to it, and a process-lifetime set would keep naming them after
// their work is gone. The panel's row is computed from the board (desktop.agentBusUnreachableWithWork).
var wakeUnreachable = struct {
	mu           sync.Mutex
	participants map[string]struct{}
}{participants: map[string]struct{}{}}

// noteWakeUnreachable records one participant once and logs it once: the point is that a departed
// session is reported at all, not that it is reported every 30 seconds.
func noteWakeUnreachable(participant string) {
	participant = strings.TrimSpace(participant)
	if participant == "" {
		return
	}
	wakeUnreachable.mu.Lock()
	defer wakeUnreachable.mu.Unlock()
	if _, seen := wakeUnreachable.participants[participant]; seen {
		return
	}
	wakeUnreachable.participants[participant] = struct{}{}
	slog.Warn("controller: agentbus wake target has no route on this host", "participant", participant,
		"unreachable", len(wakeUnreachable.participants))
}

// WakeAgentBus wakes the participants who have work waiting on them and reports
// how many were woken. The wake is keyed by the work, not by the clock, so an
// unchanged board wakes nobody however often a host ticks; a new work set wakes
// exactly once. A failed wake releases its key so the next tick retries it, and is
// recorded against the participant it could not reach. A target with no route at all — nobody
// here speaks as it and no announced address owns it — is not a failed delivery: its key stays
// claimed, so an unchanged work set is not retried every tick (which is what a board full of
// departed sessions would otherwise do forever), and it is recorded as unreachable work.
func (c *Controller) WakeAgentBus(ctx context.Context) int {
	bus, input, err := c.agentBusWakeSnapshot(ctx)
	if err != nil {
		return 0
	}
	waker := bus.currentWaker()
	if waker == nil {
		return 0
	}
	me := bus.participantID(c)
	woken := 0
	for _, target := range withPoolNotice(agentbus.WakeTargets(input), input.State, c.agentBusWakePoolAudience()) {
		// Waking ourselves is pointless: this session's next turn already carries
		// its own work.
		if target.Participant == "" || target.Participant == me {
			continue
		}
		if !bus.claimWake(target.Participant, target.Key) {
			continue
		}
		if err := waker(ctx, target); err != nil {
			if agentbus.IsNoRoute(err) {
				noteWakeUnreachable(target.Participant)
				continue
			}
			slog.Warn("controller: agentbus wake", "participant", target.Participant, "err", err)
			noteWakeFailure(target.Participant, err)
			bus.releaseWake(target.Participant, target.Key)
			continue
		}
		woken++
	}
	return woken
}

// AgentBusTick reclaims lapsed leases and then wakes whoever has work waiting: a host
// calls it on a timer, because with nothing being written nothing else would notice a
// holder that went away. The order matters — a claimed node is invisible to the wake
// surface, so the reclaim has to land before the wake is computed.
func (c *Controller) AgentBusTick(ctx context.Context) int {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return 0
	}
	if brd, err := board.Open(bus.dir); err == nil {
		if _, err := brd.Sweep(ctx, time.Now().UTC()); err != nil {
			slog.Warn("controller: agentbus reclaim on tick", "board", bus.dir, "err", err)
		}
	}
	// Talk lapses on the same tick, for the same reason: the closure is a record, not something
	// re-derived at read time, so with no writer a quiet topic stays open and keeps naming an
	// addressee with nothing left to answer (measured: five asks kept waking nobody a day later).
	c.closeLapsedAgentBusTalk(ctx)
	// Presence renews here too: a record nobody rewrites goes stale, and a stale participant
	// drops out of the roster its peers read (2026-10-05).
	c.refreshAgentBusPresence(ctx, time.Now().UTC())
	return c.WakeAgentBus(ctx)
}

func (c *Controller) agentBusWakeSnapshot(ctx context.Context) (*agentBusState, agentbus.WakeInput, error) {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	brd, err := board.Open(bus.dir)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	talkLog, err := agentbus.OpenTalkLog(bus.dir)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	talk, _, err := talkLog.Read()
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	hearingLog, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	hearings, _, err := hearingLog.Read()
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	return bus, agentbus.WakeInput{
		State:    state,
		Talk:     talk,
		Hearings: hearings,
		Limits:   bus.limitsForHearing(),
		Now:      time.Now().UTC(),
		// The same budget the dispatcher hands work out under: once a step has spent it,
		// the wake is the only thing that still mentions the step (G5).
		StallAfter: agentBusDispatchTries,
	}, nil
}

func (b *agentBusState) currentWaker() func(context.Context, agentbus.WakeTarget) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.waker
}

// WakeLedger remembers, per participant, the work set a wake was last delivered for.
// It outlives one controller: a host shares one across the controllers it rebuilds
// (a join, a tab switch, a settings change), which is what keeps the same work set
// from being delivered twice, and the per-controller map could not do that.
type WakeLedger struct {
	mu    sync.Mutex
	woken map[string]string
}

// NewWakeLedger opens an empty ledger.
func NewWakeLedger() *WakeLedger { return &WakeLedger{woken: map[string]string{}} }

// SetAgentBusWakeLedger shares a host's ledger with this controller. Without one the
// controller keeps its own, which a rebuild — exactly what enrolment does — drops.
func (c *Controller) SetAgentBusWakeLedger(ledger *WakeLedger) {
	if ledger == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.wakeLedger = ledger
	}
}

// claimWake records that this participant was woken for exactly this work set.
// One entry per participant is enough: a participant is only ever owed its newest
// wake, and the key makes an older one irrelevant.
func (b *agentBusState) claimWake(participant, key string) bool {
	b.mu.Lock()
	ledger := b.wakeLedger
	b.mu.Unlock()
	if ledger == nil {
		return true
	}
	return ledger.claim(participant, key)
}

func (b *agentBusState) releaseWake(participant, key string) {
	b.mu.Lock()
	ledger := b.wakeLedger
	b.mu.Unlock()
	if ledger != nil {
		ledger.release(participant, key)
	}
}

func (l *WakeLedger) claim(participant, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.woken[participant] == key {
		return false
	}
	l.woken[participant] = key
	return true
}

func (l *WakeLedger) release(participant, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.woken[participant] == key {
		delete(l.woken, participant)
	}
}
