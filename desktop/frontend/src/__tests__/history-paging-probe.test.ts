// Run: tsx src/__tests__/history-paging-probe.test.ts
//
// The stable channel has no opt-in diagnostics recorder, so the history-paging
// truth has to ride the settings-page export instead. This locks the probe's
// content-free fields all the way into the exported trace.
import assert from "node:assert/strict";
import { createFrontendDiagnostics, frontendDiagnosticSample } from "../lib/frontendDiagnostics";
import {
  MAX_HISTORY_REFUSALS,
  historyPagingProbeSnapshot,
  recordHistoryOlderRefusal,
  recordHistoryPaneView,
  resetHistoryPagingProbe,
} from "../lib/historyPagingProbe";

resetHistoryPagingProbe();

recordHistoryPaneView({
  splitMode: true,
  paneHasOlder: true,
  paneLoading: false,
  paneError: false,
  hasOlder: true,
  olderLoading: false,
  running: true,
  hydrating: false,
  startTurn: 4,
  totalTurns: 11,
  revision: 7,
});
recordHistoryOlderRefusal({ trigger: "viewport-user", known: true, hasOlder: true, loading: false, running: true });

// 1. The pane view survives even without a scroll element: the settings page
//    renders the recorder with no transcript, and history state is not geometry.
const sample = frontendDiagnosticSample(null, 0);
assert.ok(sample, "the sample carries history state without a scroll element");
assert.equal(sample!.historySplit, true, "split mode is recorded");
assert.equal(sample!.historyPaneHasOlder, true, "the pane's older-page flag is recorded");
assert.equal(sample!.historyOlderLoading, false, "the controller's loading flag is recorded");
assert.equal(sample!.historyRunning, true, "the run gate that defers the backfill is recorded");
assert.equal(sample!.historyStartTurn, 4, "the loaded window start is recorded");
assert.equal(sample!.historyTotalTurns, 11, "the session turn count is recorded");
assert.equal(sample!.historyRevision, 7, "the page revision is recorded");
assert.equal(sample!.historyRefusals, 1, "the refused older request is counted");
assert.equal(sample!.trigger, "viewport-user", "the refusal trigger survives the trace whitelist");

// 2. The whitelist must keep those fields in the exported payload.
let clock = 0;
const rec = createFrontendDiagnostics({ now: () => clock, randomID: () => "a".repeat(32), maxDurationMs: 120_000 });
rec.start();
clock += 60;
rec.record("history-paging", "sample", frontendDiagnosticSample(null, 0) ?? {});
const payload = rec.stop();
const recorded = payload.events.find((event) => event.type === "sample");
assert.ok(recorded, "the recorded trace carries a history sample");
assert.equal(recorded!.historyPaneHasOlder, true, "the exported trace keeps the pane flag");
assert.equal(recorded!.historyRunning, true, "the exported trace keeps the run gate");
assert.equal(recorded!.historyRefusals, 1, "the exported trace keeps the refusal count");

// 3. The ring stays bounded.
for (let index = 0; index < MAX_HISTORY_REFUSALS + 5; index += 1) {
  recordHistoryOlderRefusal({ trigger: "auto-fill", known: true, hasOlder: true, loading: false, running: false });
}
const bounded = historyPagingProbeSnapshot();
assert.equal(bounded.refusals.length, MAX_HISTORY_REFUSALS, "refusals are ring-bounded");
assert.equal(bounded.refusals.at(-1)!.trigger, "auto-fill", "the newest refusal is retained");

resetHistoryPagingProbe();
assert.equal(historyPagingProbeSnapshot().view, null, "reset clears the probe");

console.log("PASS history-paging probe: stable-channel export carries the pane's older-history truth");
