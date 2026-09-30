import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { applyAutostart, readAutostartPolicy, type LoginItemApp } from "./autostart.js";

function fakeApp(openAtLogin = false): LoginItemApp & { calls: unknown[] } {
  let registered = openAtLogin;
  const calls: unknown[] = [];
  return {
    calls,
    setLoginItemSettings(settings) {
      calls.push(settings);
      registered = settings.openAtLogin;
    },
    getLoginItemSettings() {
      return { openAtLogin: registered };
    },
  };
}

function withHome(body: (home: string) => void): void {
  const home = mkdtempSync(join(tmpdir(), "reasonix-autostart-"));
  try {
    body(home);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
}

test("no policy file means no login item is registered", () => {
  withHome((home) => {
    assert.equal(readAutostartPolicy(home).enabled, false);
    const app = fakeApp();
    assert.equal(applyAutostart(app, home), false);
    assert.equal(app.calls.length, 0, "an unasked machine must never be touched");
  });
});

test("the policy file registers the login item", () => {
  withHome((home) => {
    writeFileSync(join(home, "desktop-autostart.json"), JSON.stringify({ enabled: true }));
    const app = fakeApp();
    assert.equal(applyAutostart(app, home), true);
    assert.deepEqual(app.calls, [{ openAtLogin: true, openAsHidden: true, args: ["--unattended-boot"] }]);
  });
});

test("disabling removes a previously registered login item", () => {
  withHome((home) => {
    writeFileSync(join(home, "desktop-autostart.json"), JSON.stringify({ enabled: false }));
    const app = fakeApp(true);
    assert.equal(applyAutostart(app, home), false);
    assert.equal(app.calls.length, 1);
    assert.equal((app.calls[0] as { openAtLogin: boolean }).openAtLogin, false);
  });
});

test("a matching registration is left alone", () => {
  withHome((home) => {
    writeFileSync(join(home, "desktop-autostart.json"), JSON.stringify({ enabled: true }));
    const app = fakeApp(true);
    assert.equal(applyAutostart(app, home), true);
    assert.equal(app.calls.length, 0, "no write when the machine already matches the policy");
  });
});

test("a damaged policy file reads as disabled", () => {
  withHome((home) => {
    writeFileSync(join(home, "desktop-autostart.json"), "{not json");
    assert.equal(readAutostartPolicy(home).enabled, false);
  });
});
