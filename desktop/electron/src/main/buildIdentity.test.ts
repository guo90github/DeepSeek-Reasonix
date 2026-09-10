import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { shellBuildIdentity } from "./buildIdentity.js";

const resources = () => mkdtempSync(join(tmpdir(), "reasonix-build-identity-"));

test("packaged shell reports the build.json identity the service compares", () => {
  const root = resources();
  writeFileSync(
    join(root, "build.json"),
    JSON.stringify({ schemaVersion: 1, version: "v0.0.0-dev", channel: "stable", commit: "e68d3e5b4fd7" }),
  );
  assert.deepEqual(shellBuildIdentity({ packaged: true, resourcesPath: root, fallbackVersion: "0.0.0" }), {
    version: "v0.0.0-dev",
    channel: "stable",
    commit: "e68d3e5b4fd7",
  });
});

test("packaged shell without build.json keeps its own version instead of claiming dev", () => {
  const root = resources();
  assert.deepEqual(shellBuildIdentity({ packaged: true, resourcesPath: root, fallbackVersion: "0.0.0" }), {
    version: "0.0.0",
    channel: "dev",
    commit: "dev",
  });
});

test("packaged shell ignores malformed build.json fields", () => {
  const root = resources();
  writeFileSync(join(root, "build.json"), JSON.stringify({ version: 7, channel: "", commit: "abc" }));
  assert.deepEqual(shellBuildIdentity({ packaged: true, resourcesPath: root, fallbackVersion: "0.0.0" }), {
    version: "0.0.0",
    channel: "dev",
    commit: "abc",
  });
});

test("unpackaged shell stays dev and honours the environment overrides", () => {
  const identity = shellBuildIdentity({
    packaged: false,
    resourcesPath: "/nonexistent",
    fallbackVersion: "0.0.0",
    env: { REASONIX_CHANNEL: "preview", REASONIX_COMMIT: "abc123" },
  });
  assert.deepEqual(identity, { version: "dev", channel: "preview", commit: "abc123" });
});
