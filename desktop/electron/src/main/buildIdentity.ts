import { readFileSync } from "node:fs";
import { join } from "node:path";

// The packaged shell must report the identity package.mjs wrote into
// resources/build.json: the service compares it byte-for-byte during
// desktop/hello, while app.getVersion() only carries packager's numeric
// appVersion (v1.2.3-rc.1 becomes 1.2.3).
export interface ShellBuildIdentity {
  version: string;
  channel: string;
  commit: string;
}

export function shellBuildIdentity(input: {
  packaged: boolean;
  resourcesPath: string;
  fallbackVersion: string;
  env?: Record<string, string | undefined>;
}): ShellBuildIdentity {
  const env = input.env ?? {};
  if (!input.packaged) {
    return { version: "dev", channel: env.REASONIX_CHANNEL || "dev", commit: env.REASONIX_COMMIT || "dev" };
  }
  const build = readBuildInfo(join(input.resourcesPath, "build.json"));
  return {
    version: build.version ?? input.fallbackVersion,
    channel: build.channel || env.REASONIX_CHANNEL || "dev",
    commit: build.commit || env.REASONIX_COMMIT || "dev",
  };
}

function readBuildInfo(path: string): { version?: string; channel?: string; commit?: string } {
  try {
    const parsed = JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>;
    const field = (key: string) => (typeof parsed[key] === "string" && parsed[key] !== "" ? (parsed[key] as string) : undefined);
    return { version: field("version"), channel: field("channel"), commit: field("commit") };
  } catch {
    return {};
  }
}
