// The assertion idiom the frontend suites repeat dozens of times: ok/eq plus one tally and one
// summary line. Suites that adopt this file stop carrying their own copy; the ones that have
// not yet are unchanged.

let passed = 0;
let failed = 0;

export function ok(value: boolean, label: string): void {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

export function eq(actual: unknown, expected: unknown, label: string): void {
  if (actual === expected) {
    ok(true, label);
  } else {
    ok(false, `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

/** Prints the suite tally and exits non-zero when anything failed. */
export function suiteSummary(): void {
  console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
  if (failed > 0) process.exit(1);
}
