#!/usr/bin/env node
// Runs the frontend build for one shell. The shell name travels through the
// environment because npm scripts cannot set variables portably on Windows.
import { spawnSync } from "node:child_process";
import { shellFromEnv } from "./shell-css.mjs";

const shell = shellFromEnv({ REASONIX_SHELL: process.argv[2] ?? "" });
// Local-only knob: `pnpm build` also runs eslint, tsc and the contract checks,
// which gate CI rather than the artifact. `vite build` is the only step that
// writes one, so it stays.
const args = process.env.REASONIX_LOCAL_SKIP_CHECKS === "1" ? ["exec", "vite", "build"] : ["build"];
const result = spawnSync("pnpm", args, {
  stdio: "inherit",
  env: { ...process.env, REASONIX_SHELL: shell },
  shell: process.platform === "win32",
});
process.exit(result.status ?? 1);
