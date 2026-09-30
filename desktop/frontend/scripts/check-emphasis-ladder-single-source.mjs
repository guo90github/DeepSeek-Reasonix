#!/usr/bin/env node
// The answer-emphasis ladder must stay single-sourced in the app-shell
// stylesheet. The one time it was copied into a feature stylesheet the two
// drifted, so each anchor is an exact declaration string: a re-implementation
// fails here instead of silently diverging.

import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const sourceRoot = path.resolve(scriptDir, "..", "src");
const HOME = "styles.css";

const anchors = new Map([
  ["marker-pen band", "transparent 50%, color-mix(in srgb, var(--accent) 48%, transparent) 58%"],
  ["h1 rail", "color-mix(in srgb, var(--accent) 72%, transparent), color-mix(in srgb, var(--warn) 45%, transparent)"],
  ["labeled-bullet rail", ".md li.md-li--label"],
  ["headline claim block", ".md p.md-p--claim"],
]);

function stylesheets(dir, found = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) stylesheets(full, found);
    else if (entry.name.endsWith(".css")) found.push(full);
  }
  return found;
}

const files = stylesheets(sourceRoot);
const owned = new Map(files.map((file) => [file, readFileSync(file, "utf8")]));
const offenders = [];

for (const [label, anchor] of anchors) {
  const homes = files.filter((file) => owned.get(file).includes(anchor)).map((file) => path.relative(sourceRoot, file));
  if (homes.length !== 1 || homes[0] !== HOME) {
    offenders.push(`- ${label} ${JSON.stringify(anchor)}: found in ${homes.length ? homes.join(", ") : "nowhere"}, expected only ${HOME}`);
  }
}

if (offenders.length > 0) {
  console.error("Emphasis ladder must stay single-sourced:");
  for (const line of offenders) console.error(line);
  process.exit(1);
}
console.log(`emphasis ladder single-source check passed (${anchors.size} anchors across ${files.length} stylesheets)`);
