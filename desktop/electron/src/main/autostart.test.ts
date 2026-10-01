import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
  AUTOSTART_FILE,
  applyAutostart,
  loginItemTarget,
  readAutostartPolicy,
  type LoginItemApp,
} from "./autostart.js";

function fakeApp(): LoginItemApp & { calls: unknown[] } {
  const calls: unknown[] = [];
  return {
    calls,
    setLoginItemSettings(settings) {
      calls.push(settings);
    },
  };
}

/** An executable with no install root above it, so no target is on offer. */
const FLAT_EXE = join(tmpdir(), "reasonix-unversioned", "app", "Reasonix.exe");

function withHome(body: (home: string) => void): void {
  const home = mkdtempSync(join(tmpdir(), "reasonix-autostart-"));
  try {
    body(home);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
}

function withVersionedInstall(body: (root: string, execPath: string) => void): void {
  const root = mkdtempSync(join(tmpdir(), "reasonix-install-"));
  try {
    pointAt(root, "v1.2.3");
    const execPath = join(root, "versions", "v1.2.3", "app", "Reasonix.exe");
    mkdirSync(join(root, "versions", "v1.2.3", "app"), { recursive: true });
    writeFileSync(execPath, "");
    body(root, execPath);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

/** Writes current.json the way the portable installer and updater do. */
function pointAt(root: string, version: string): void {
  const pointer = { schemaVersion: 1, activeVersion: version, activeDir: `versions/${version}` };
  writeFileSync(join(root, "current.json"), JSON.stringify(pointer));
}

/** Creates a version directory's desktop binary — the legal service entry point. */
function writeDesktop(root: string, version: string): string {
  const name = process.platform === "win32" ? "reasonix-desktop.exe" : "reasonix-desktop";
  const desktop = join(root, "versions", version, name);
  mkdirSync(join(root, "versions", version), { recursive: true });
  writeFileSync(desktop, "");
  return desktop;
}

function launcherName(): string {
  return process.platform === "win32" ? "Reasonix.exe" : "reasonix-launcher";
}

function fallbackLauncherName(): string {
  return process.platform === "win32" ? "reasonix-launcher.exe" : "reasonix-launcher";
}

test("no policy file means no login item is registered", () => {
  withHome((home) => {
    const policy = readAutostartPolicy(home);
    assert.equal(policy.present, false);
    assert.equal(policy.enabled, false);
    const app = fakeApp();
    assert.equal(applyAutostart(app, home, FLAT_EXE), false);
    assert.equal(app.calls.length, 0, "an unasked machine must never be touched");
  });
});

test("the policy file registers the login item", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: true }));
    const app = fakeApp();
    assert.equal(applyAutostart(app, home, FLAT_EXE), true);
    assert.deepEqual(app.calls, [{ openAtLogin: true, openAsHidden: true, args: ["--unattended-boot"] }]);
  });
});

test("disabling removes a previously registered login item", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: false }));
    const app = fakeApp();
    assert.equal(applyAutostart(app, home, FLAT_EXE), false);
    assert.deepEqual(app.calls, [{ openAtLogin: false }]);
  });
});

test("an enabled policy rewrites the entry on every launch", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: true }));
    const app = fakeApp();
    applyAutostart(app, home, FLAT_EXE);
    applyAutostart(app, home, FLAT_EXE);
    assert.equal(app.calls.length, 2, "the registered entry is never trusted to be current");
  });
});

test("a policy file with custom args keeps them", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: true, args: ["--unattended-boot", "--quiet"] }));
    const app = fakeApp();
    assert.equal(applyAutostart(app, home, FLAT_EXE), true);
    assert.deepEqual(app.calls, [{ openAtLogin: true, openAsHidden: true, args: ["--unattended-boot", "--quiet"] }]);
  });
});

test("a damaged policy file is not a registration instruction", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), "{not json");
    const policy = readAutostartPolicy(home);
    assert.equal(policy.present, false);
    assert.equal(policy.enabled, false);
    const app = fakeApp();
    assert.equal(applyAutostart(app, home, FLAT_EXE), false);
    assert.equal(app.calls.length, 0);
  });
});

test("a versioned install registers the active version's desktop binary", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: true }));
    withVersionedInstall((root, execPath) => {
      const desktop = writeDesktop(root, "v1.2.3");
      assert.equal(loginItemTarget(execPath), desktop);

      const app = fakeApp();
      assert.equal(applyAutostart(app, home, execPath), true);
      assert.deepEqual(app.calls, [{ openAtLogin: true, openAsHidden: true, args: ["--unattended-boot"], path: desktop }]);
    });
  });
});

test("a version bump moves the login item to the new version directory", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: true }));
    withVersionedInstall((root, execPath) => {
      const first = writeDesktop(root, "v1.2.3");
      assert.equal(applyAutostart(fakeApp(), home, execPath), true);

      pointAt(root, "v1.2.4");
      const second = writeDesktop(root, "v1.2.4");
      const app = fakeApp();
      assert.equal(applyAutostart(app, home, execPath), true);
      assert.notEqual(first, second);
      assert.deepEqual(app.calls, [{ openAtLogin: true, openAsHidden: true, args: ["--unattended-boot"], path: second }]);
    });
  });
});

test("a versioned install without the desktop binary falls back to the launcher", () => {
  withVersionedInstall((root, execPath) => {
    const launcher = join(root, launcherName());
    writeFileSync(launcher, "");
    assert.equal(loginItemTarget(execPath), launcher);
  });
});

test("the launcher binary is the fallback when the portable alias is absent", () => {
  withVersionedInstall((root, execPath) => {
    const fallback = join(root, fallbackLauncherName());
    writeFileSync(fallback, "");
    assert.equal(loginItemTarget(execPath), fallback);
  });
});

test("a flat install with no target keeps the running executable", () => {
  withHome((home) => {
    writeFileSync(join(home, AUTOSTART_FILE), JSON.stringify({ enabled: true }));
    withVersionedInstall((_root, execPath) => {
      assert.equal(loginItemTarget(execPath), null);
      const app = fakeApp();
      assert.equal(applyAutostart(app, home, execPath), true);
      assert.deepEqual(app.calls, [{ openAtLogin: true, openAsHidden: true, args: ["--unattended-boot"] }]);
    });
  });
});
