import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { readHostState, unattendedDesired } from "./hostState.js";

function withHome(body: (home: string) => void): void {
  const home = mkdtempSync(join(tmpdir(), "reasonix-host-state-"));
  try {
    body(home);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
}

test("a clean exit leaves no marker, so nothing claims unattended", () => {
  withHome((home) => {
    assert.equal(readHostState(home), null);
    assert.equal(unattendedDesired(home), false);
  });
});

test("the marker's unattended flag drives the restart policy", () => {
  withHome((home) => {
    writeFileSync(
      join(home, "desktop-host-state.json"),
      JSON.stringify({ schemaVersion: 1, pid: 4242, phase: "running", unattended: true }),
    );
    assert.equal(unattendedDesired(home), true);
    assert.equal(readHostState(home)?.pid, 4242);
  });
});

test("an attended marker is not mistaken for an unattended one", () => {
  withHome((home) => {
    writeFileSync(
      join(home, "desktop-host-state.json"),
      JSON.stringify({ schemaVersion: 1, pid: 4242, phase: "running", unattended: false }),
    );
    assert.equal(unattendedDesired(home), false);
  });
});

test("a damaged or foreign marker reads as attended", () => {
  withHome((home) => {
    writeFileSync(join(home, "desktop-host-state.json"), "{not json");
    assert.equal(unattendedDesired(home), false);
    writeFileSync(join(home, "desktop-host-state.json"), JSON.stringify({ schemaVersion: 1, pid: 1 }));
    assert.equal(unattendedDesired(home), false);
  });
});

test("an empty home never reports unattended", () => {
  assert.equal(unattendedDesired(""), false);
});
