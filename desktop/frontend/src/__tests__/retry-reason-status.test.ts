// Run: tsx src/__tests__/retry-reason-status.test.ts

import { recoveryStatusText, retryReasonLabel, type RecoveryRetry } from "../lib/recoveryStatus";
import type { Translator } from "../lib/i18n";
import { en } from "../locales/en";
import { zh } from "../locales/zh";
import { zhTW } from "../locales/zh-TW";

let failed = 0;

function ok(value: unknown, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

// A stub that echoes the key (and its variables) so the assertions are about which copy was
// chosen, not about the wording of any one locale.
const t = ((key: string, vars?: Record<string, string | number>) =>
  vars ? `${key} ${JSON.stringify(vars)}` : key) as unknown as Translator;

const base = t("status.retrying", { attempt: 3, max: 10 });

console.log("\nretry status names the reason the host labelled");

const labelled: [string, "status.retryReasonRateLimited" | "status.retryReasonServer" | "status.retryReasonTimeout" | "status.retryReasonNetwork"][] = [
  ["rate_limited", "status.retryReasonRateLimited"],
  ["server_error", "status.retryReasonServer"],
  ["timeout", "status.retryReasonTimeout"],
  ["network", "status.retryReasonNetwork"],
];
for (const [reason, key] of labelled) {
  const retry: RecoveryRetry = { attempt: 3, max: 10, reason };
  ok(recoveryStatusText(t, retry, 0) === `${base} ${key}`, `${reason} is named beside the attempt count`);
  ok(retryReasonLabel(t, reason) === key, `${reason} maps to its own copy`);
}

// A reason this build does not know — and no reason at all — adds nothing: "retrying (3/10)"
// is honest, a wrong label is not.
ok(recoveryStatusText(t, { attempt: 3, max: 10, reason: "something_new" }, 0) === base, "an unknown reason adds no label");
ok(recoveryStatusText(t, { attempt: 3, max: 10 }, 0) === base, "a missing reason adds no label");
ok(retryReasonLabel(t, undefined) === "", "no reason translates to nothing");

// While the host is waiting out a recovery window the banner speaks for itself.
const waiting = recoveryStatusText(
  t,
  { attempt: 3, max: 10, reason: "rate_limited", recovery: { waiting: true, phase: "connect", next_attempt_at: 6_000 } },
  0,
);
ok(waiting.includes("status.recoveryWaiting"), "the recovery-wait banner is left as it was");

// Every label exists in all three locales, so a Chinese UI never falls back to English.
for (const key of ["status.retryReasonRateLimited", "status.retryReasonServer", "status.retryReasonTimeout", "status.retryReasonNetwork"] as const) {
  ok(Boolean(en[key]) && Boolean(zh[key]) && Boolean(zhTW[key]), `${key} is present in en, zh and zh-TW`);
}

console.log(`\n${failed === 0 ? "OK" : "FAILED"}: ${failed} failed`);
if (failed > 0) process.exit(1);
