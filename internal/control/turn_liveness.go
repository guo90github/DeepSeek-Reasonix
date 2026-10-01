package control

import (
	"fmt"
	"sync/atomic"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// turnStallThreshold is the silence after which a running turn is reported as
// possibly stuck. It only warns by default: the user decides whether to stop,
// because a legitimately long tool and a wedged one look identical from here.
// Unattended driving reuses the same bound to cancel such a turn.
var turnStallThreshold atomic.Int64

func init() { turnStallThreshold.Store(int64(10 * time.Minute)) }

// TurnStallThreshold is that bound, so a driver outside this package cancels a
// wedged turn at exactly the silence the notice calls stuck.
func TurnStallThreshold() time.Duration { return time.Duration(turnStallThreshold.Load()) }

// turnLiveness remembers the last event a running turn produced so a silent
// stretch can be surfaced instead of leaving "working" unexplained. It also
// remembers a turn that died on the provider's context window: when the model
// never reported a window size, that failure is the only signal a driver has
// that the session is spent.
type turnLiveness struct {
	lastEvent atomic.Int64
	warned    atomic.Bool
	exhausted atomic.Bool
}

func (l *turnLiveness) reset(now time.Time) {
	l.lastEvent.Store(now.UnixNano())
	l.warned.Store(false)
	l.exhausted.Store(false)
}

func (l *turnLiveness) observe(e event.Event, now time.Time) {
	if e.Kind == event.Notice && e.Code == event.NoticeCodeTurnStalled {
		return
	}
	l.lastEvent.Store(now.UnixNano())
	l.warned.Store(false)
}

// stalledFor claims the single warning for the current silence.
func (l *turnLiveness) stalledFor(now time.Time) (time.Duration, bool) {
	last := l.lastEvent.Load()
	if last == 0 {
		return 0, false
	}
	silence := now.Sub(time.Unix(0, last))
	if silence < TurnStallThreshold() {
		return 0, false
	}
	return silence, l.warned.CompareAndSwap(false, true)
}

// noteContextExhaustion records a turn that died on the provider's window. The
// next turn's reset clears it.
func (l *turnLiveness) noteContextExhaustion(err error) {
	if provider.AsContextLimitError(err) != nil {
		l.exhausted.Store(true)
	}
}

func (c *Controller) warnIfTurnStalled(now time.Time) {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	if !running {
		return
	}
	silence, ok := c.liveness.stalledFor(now)
	if !ok {
		return
	}
	c.sink.Emit(event.Event{
		Kind:  event.Notice,
		Code:  event.NoticeCodeTurnStalled,
		Level: event.LevelWarn,
		Text:  fmt.Sprintf("No progress for %s. The turn is still running; press Stop if it looks stuck.", silence.Round(time.Minute)),
	})
}

// TurnSilence reports how long the running turn has been silent, and whether a
// turn is running at all. Reading it changes nothing: a driver outside this
// package decides what a silent turn deserves.
func (c *Controller) TurnSilence(now time.Time) (time.Duration, bool) {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	if !running {
		return 0, false
	}
	last := c.liveness.lastEvent.Load()
	if last == 0 {
		return 0, false
	}
	return now.Sub(time.Unix(0, last)), true
}

// ContextExhausted reports that the last turn of this session failed on the
// provider's context window.
func (c *Controller) ContextExhausted() bool { return c.liveness.exhausted.Load() }
