import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
  defaultElectronCacheRoot,
  electronZipNames,
  findElectronZipDir,
  parseTarget,
  resolveElectronZipDir,
} from "./lib.mjs";

// packager's SHASUMS256.txt check ignores @electron/get's cache and hits the
// network on every run, so a flaky connection fails a build whose zip is
// already on disk (2026-10-03: two builds died on ECONNRESET for exactly that).
// These helpers point packaging at the cached zip by default.

test("the cache root follows @electron/get's per-platform default", () => {
  const win = { env: { LOCALAPPDATA: "C:\\Local" }, platform: "win32", home: "C:\\Users\\me" };
  assert.equal(defaultElectronCacheRoot(win), join("C:\\Local", "electron", "Cache"));
  assert.equal(defaultElectronCacheRoot({ env: {}, platform: "win32", home: "C:\\Users\\me" }), join("C:\\Users\\me", "AppData", "Local", "electron", "Cache"));
  assert.equal(defaultElectronCacheRoot({ env: {}, platform: "darwin", home: "/Users/me" }), join("/Users/me", "Library", "Caches", "electron", "Cache"));
  assert.equal(defaultElectronCacheRoot({ env: {}, platform: "linux", home: "/home/me" }), join("/home/me", ".cache", "electron", "Cache"));
  assert.equal(defaultElectronCacheRoot({ env: { XDG_CACHE_HOME: "/xdg" }, platform: "linux", home: "/home/me" }), join("/xdg", "electron", "Cache"));
  assert.equal(defaultElectronCacheRoot({ env: { ELECTRON_CACHE: "/electron" }, platform: "linux", home: "/home/me" }), "/electron");
});

test("a target needs every zip @electron/get would fetch for it", () => {
  assert.deepEqual(electronZipNames({ target: parseTarget("windows/amd64"), electronVersion: "44.2.0" }), ["electron-v44.2.0-win32-x64.zip"]);
  assert.deepEqual(electronZipNames({ target: parseTarget("linux/arm64"), electronVersion: "44.2.0" }), ["electron-v44.2.0-linux-arm64.zip"]);
  assert.deepEqual(electronZipNames({ target: parseTarget("darwin/universal"), electronVersion: "44.2.0" }), [
    "electron-v44.2.0-darwin-arm64.zip",
    "electron-v44.2.0-darwin-x64.zip",
  ]);
});

test("packaging resolves the cached zip, and only when every zip is there", () => {
  const cache = mkdtempSync(join(tmpdir(), "electron-cache-"));
  try {
    const target = parseTarget("windows/amd64");
    const zipNames = electronZipNames({ target, electronVersion: "44.2.0" });
    assert.equal(resolveElectronZipDir({ target, electronVersion: "44.2.0", env: {}, cacheRoot: cache }), undefined, "an empty cache keeps the download path");

    const hashed = join(cache, "073c10f139c87e3b");
    mkdirSync(hashed);
    writeFileSync(join(hashed, "electron-v44.2.0-win32-x64.zip"), "");
    assert.equal(findElectronZipDir({ zipNames, cacheRoot: cache }), hashed, "@electron/get's hashed subdirectory is found");
    assert.equal(resolveElectronZipDir({ target, electronVersion: "44.2.0", env: {}, cacheRoot: cache }), hashed, "no env var needed: the cache is the default");
    assert.equal(resolveElectronZipDir({ target, electronVersion: "44.2.0", env: { REASONIX_ELECTRON_ZIP_DIR: "/explicit" }, cacheRoot: cache }), "/explicit", "an explicit directory still wins");
    assert.equal(resolveElectronZipDir({ target, electronVersion: "44.2.0", env: { REASONIX_ELECTRON_ZIP_DIR: "  " }, cacheRoot: cache }), hashed, "a blank override falls back to the cache");

    assert.equal(resolveElectronZipDir({ target: parseTarget("darwin/universal"), electronVersion: "44.2.0", env: {}, cacheRoot: cache }), undefined, "a target whose second zip is missing keeps the download path");
    writeFileSync(join(hashed, "electron-v44.2.0-darwin-arm64.zip"), "");
    writeFileSync(join(hashed, "electron-v44.2.0-darwin-x64.zip"), "");
    assert.equal(resolveElectronZipDir({ target: parseTarget("darwin/universal"), electronVersion: "44.2.0", env: {}, cacheRoot: cache }), hashed, "both universal zips in one directory resolve");

    const flat = join(cache, "flat");
    mkdirSync(flat);
    writeFileSync(join(flat, "electron-v45.0.0-win32-x64.zip"), "");
    assert.equal(resolveElectronZipDir({ target, electronVersion: "45.0.0", env: {}, cacheRoot: cache }), flat, "zips kept directly in the root resolve too");
    assert.equal(resolveElectronZipDir({ target, electronVersion: "46.0.0", env: {}, cacheRoot: cache }), undefined, "a version the cache does not hold keeps the download path");
  } finally {
    rmSync(cache, { recursive: true, force: true });
  }
});
