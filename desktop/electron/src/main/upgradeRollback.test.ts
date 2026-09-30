import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { findInstallRoot, readUpgradeState, rollbackUpgrade } from "./upgradeRollback.js";

function withHome(body: (home: string) => void): void {
  const home = mkdtempSync(join(tmpdir(), "reasonix-upgrade-"));
  try {
    body(home);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
}

function seedInstall(activatedAt: string, phase = "pending"): { home: string; root: string; exec: string } {
  const base = mkdtempSync(join(tmpdir(), "reasonix-install-"));
  const root = join(base, "Reasonix-portable");
  const home = join(base, "home");
  mkdirSync(join(root, "versions", "v0.0.0-dev.91"), { recursive: true });
  mkdirSync(join(root, "versions", "v0.0.0-dev.92"), { recursive: true });
  mkdirSync(home, { recursive: true });
  writeFileSync(
    join(root, "current.json"),
    JSON.stringify({ schemaVersion: 1, activeVersion: "v0.0.0-dev.92", activeDir: "versions/v0.0.0-dev.92" }),
  );
  writeFileSync(
    join(home, "desktop-upgrade.json"),
    JSON.stringify({ schemaVersion: 1, phase, from: "v0.0.0-dev.91", to: "v0.0.0-dev.92", activatedAt }),
  );
  const exec = join(root, "versions", "v0.0.0-dev.92", "reasonix-desktop.exe");
  return { home, root, exec };
}

test("a missing or damaged switch state rolls nothing back", () => {
  withHome((home) => {
    assert.equal(readUpgradeState(home), null);
    assert.equal(rollbackUpgrade(home, "/nowhere/reasonix-desktop.exe"), null);
    writeFileSync(join(home, "desktop-upgrade.json"), "{not json");
    assert.equal(rollbackUpgrade(home, "/nowhere/reasonix-desktop.exe"), null);
  });
});

test("a young switch is left alone so the new version can boot", () => {
  const { home, exec } = seedInstall(new Date().toISOString());
  try {
    assert.equal(rollbackUpgrade(home, exec), null);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("a stale pending switch restores the previous pointer and its entry point", () => {
  const { home, root, exec } = seedInstall(new Date(Date.now() - 30 * 60 * 1000).toISOString());
  try {
    const relaunch = rollbackUpgrade(home, exec);
    assert.equal(relaunch, join(root, "versions", "v0.0.0-dev.91", "reasonix-desktop.exe"));
    const pointer = JSON.parse(readFileSync(join(root, "current.json"), "utf8")) as {
      activeVersion: string;
      activeDir: string;
    };
    assert.equal(pointer.activeVersion, "v0.0.0-dev.91");
    assert.equal(pointer.activeDir, "versions/v0.0.0-dev.91");
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("a healthy switch is never rolled back", () => {
  const { home, exec } = seedInstall(new Date(Date.now() - 30 * 60 * 1000).toISOString(), "healthy");
  try {
    assert.equal(rollbackUpgrade(home, exec), null);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("an install without current.json is not a versioned one", () => {
  withHome((home) => {
    assert.equal(findInstallRoot(join(home, "somewhere", "reasonix-desktop.exe")), null);
  });
});

test("the install root is found by climbing out of a versions tree", () => {
  const { home, root, exec } = seedInstall(new Date().toISOString());
  try {
    assert.equal(findInstallRoot(exec), root);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});
