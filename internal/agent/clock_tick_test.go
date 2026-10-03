package agent

import (
	"time"
)

// The incident state only counts an observation that is strictly later than the last one, which
// is what keeps a stale healthy turn from clearing a newer incident. Observations inside one
// timer tick are indistinguishable, and the clock granularity here is coarse enough to hit that.
func waitForDistinctClockTick() {
	before := time.Now()
	for !time.Now().After(before) {
		time.Sleep(time.Millisecond)
	}
}
