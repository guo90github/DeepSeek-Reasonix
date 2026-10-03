// Pure value normalisation the settings panel shares with its own sections: header text ↔ record,
// a key-sorted JSON projection, and the reasoning-language tag. None of them touch React state.

export function formatProviderHeaders(headers: Record<string, string> | null | undefined): string {
  const entries = Object.entries(headers ?? {})
    .map(([key, value]) => [key.trim(), String(value ?? "").trim()] as const)
    .filter(([key, value]) => key && value)
    .sort(([a], [b]) => a.localeCompare(b));
  return entries.map(([key, value]) => `${key}: ${value}`).join("\n");
}

export function parseProviderHeaders(raw: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const colon = trimmed.indexOf(":");
    const eq = trimmed.indexOf("=");
    const cut = colon >= 0 && (eq < 0 || colon < eq) ? colon : eq;
    if (cut <= 0) continue;
    const key = trimmed.slice(0, cut).trim();
    const value = trimmed.slice(cut + 1).trim();
    if (key && value) out[key] = value;
  }
  return out;
}

export function sortedJSONValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortedJSONValue);
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const key of Object.keys(value as Record<string, unknown>).sort((a, b) => a.localeCompare(b))) {
      out[key] = sortedJSONValue((value as Record<string, unknown>)[key]);
    }
    return out;
  }
  return value;
}

export function normalizeReasoningLanguage(lang: string | undefined): string {
  const v = String(lang ?? "").trim().toLowerCase();
  return v === "zh" || v === "en" ? v : "auto";
}
