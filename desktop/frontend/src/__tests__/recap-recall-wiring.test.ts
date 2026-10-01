// Run: npx tsx src/__tests__/recap-recall-wiring.test.ts
//
// The recap page's recall strip is an *optional* prop, so dropping it from the
// overlay command table is legal to the type checker and invisible at runtime —
// which is exactly how the strip shipped unreachable (U-4). This asserts the four
// links from the host call to the page's prop stay connected.

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

let failed = 0;

function ok(value: boolean, label: string) {
  if (value) process.stdout.write(`  PASS  ${label}\n`);
  else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const testDir = fileURLToPath(new URL(".", import.meta.url));
const source = (relative: string) => readFileSync(resolve(testDir, relative), "utf8");

const skillActions = source("../lib/recapSkillActions.ts");
const controller = source("../lib/useController.ts");
const adapter = source("../app-runtime/useAppRuntimeAdapter.ts");
const builders = source("../app-shell/overlayBuilders.ts");
const view = source("../app-shell/AppRuntimeView.tsx");

ok(skillActions.includes("app.RecallRecordForSession(sessionPath)"),
  "the recap action hook reads a session's recall record by transcript path");
ok(controller.includes("listRecapInsights, recallRecordForSession,"),
  "the controller hands that reader to the frontends");
ok(adapter.includes("recallRecordForSession: controller.recallRecordForSession,"),
  "the runtime adapter forwards it");
ok(view.includes("recallRecordForSession: runtime.sessionActions.recallRecordForSession,"),
  "the app view forwards it into the overlay builder's input");
ok(builders.includes("recallRecord: sessionActions.recallRecordForSession"),
  "the recap overlay passes it to the page, which is what makes the strip reachable");

process.stdout.write(`\nrecap recall wiring: ${failed} failed\n`);
if (failed > 0) process.exit(1);
