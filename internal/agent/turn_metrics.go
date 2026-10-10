package agent

import (
	"fmt"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
)

// A turn reports its metrics only when it ran long enough for the user to have
// wondered where the time went. Rounds are the multiplier, so five of them — or
// half a minute of wall clock — is where the readout starts being useful.
const (
	turnMetricsMinRounds = 5
	turnMetricsMinWall   = 30 * time.Second
)

func turnMetricsWorthReporting(rounds int, wall time.Duration) bool {
	return rounds >= turnMetricsMinRounds || wall >= turnMetricsMinWall
}

// emitTurnMetrics tells the user where a long turn's time went. The numbers come
// from the turn's own budget: batches never overlap, so the tool windows summed
// there are the turn's tool critical path and the model owns the rest.
func (a *Agent) emitTurnMetrics() {
	if a == nil || a.svc.sink == nil {
		return
	}
	rounds := a.turn.budget.rounds
	wall := a.turn.budget.elapsed()
	if !turnMetricsWorthReporting(rounds, wall) {
		return
	}
	tools := time.Duration(a.turn.budget.toolBusyMs) * time.Millisecond
	a.svc.sink.Emit(event.Event{
		Kind:   event.Notice,
		Level:  event.LevelInfo,
		Code:   event.NoticeCodeTurnMetrics,
		Text:   i18n.M.TurnMetrics,
		Detail: turnMetricsDetail(rounds, wall, tools),
	})
}

// turnMetricsDetail renders the numbers language-neutrally, so a frontend that
// does not know the code still shows something readable. The two shares are
// complements of one another: a readout whose parts do not add up to the whole
// is a bug the user can see.
func turnMetricsDetail(rounds int, wall, tools time.Duration) string {
	if tools < 0 {
		tools = 0
	}
	if wall < 0 {
		wall = 0
	}
	if tools > wall {
		tools = wall
	}
	model := wall - tools
	toolsShare := 0
	if wall > 0 {
		toolsShare = int(tools * 100 / wall)
	}
	modelShare := 0
	if wall > 0 {
		modelShare = 100 - toolsShare
	}
	return fmt.Sprintf("rounds=%d wall=%s model=%s (%d%%) tools=%s (%d%%)",
		rounds, wall.Round(time.Second), model.Round(time.Second), modelShare,
		tools.Round(time.Second), toolsShare)
}
