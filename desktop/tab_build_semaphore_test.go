package main

import (
	"context"
	"testing"
	"time"
)

// TestTabControllerBuildsSerializeOnConcurrencyLimit pins that a limit of one
// lets only the first build run; the second is held at the semaphore (it never
// reaches the start hook) until the first build finishes and releases its slot.
func TestTabControllerBuildsSerializeOnConcurrencyLimit(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	app.ctx = context.Background()
	app.readyHook = func() {}
	installNoopRuntimeEvents(app)

	original := tabBuildSlots
	tabBuildSlots = make(chan struct{}, 1)
	t.Cleanup(func() { tabBuildSlots = original })

	gate := newTabBuildGate(app)
	t.Cleanup(func() {
		gate.releaseAll()
		app.shutdown(context.Background())
	})

	tabA := app.createTabEntryWithID("global", "", "", "slot-a")
	tabB := app.createTabEntryWithID("global", "", "", "slot-b")
	app.mu.Lock()
	app.tabs[tabA.ID] = tabA
	app.tabs[tabB.ID] = tabB
	app.tabOrder = []string{tabA.ID, tabB.ID}
	app.activeTabID = tabA.ID
	app.mu.Unlock()

	app.startTabControllerBuild(tabA)
	gate.waitEntered(t, tabA.ID) // A holds the single slot and is gated

	app.startTabControllerBuild(tabB)

	// B must be blocked at the semaphore, not at the start hook.
	select {
	case got := <-gate.entered:
		t.Fatalf("build for tab %q reached the start hook while A held the slot", got)
	case <-time.After(500 * time.Millisecond):
	}

	gate.release(tabA.ID)        // A's build finishes and frees the slot
	gate.waitEntered(t, tabB.ID) // B then acquires and reaches the gate
	gate.release(tabB.ID)
}
