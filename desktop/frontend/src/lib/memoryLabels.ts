// Labels and formatters the memory panel shares with its sub-views: a fact's display title,
// key, freshness/scope/type/doc copy, and the timestamp formatting. None of them render, so
// they live apart from the panel that uses them.

import type { MemoryArchive, MemoryFact, MemorySuggestionsView } from "./types";
import type { useT } from "./i18n";

type Translator = ReturnType<typeof useT>;

export type LinkInfo = {
  name: string;
  exists: boolean;
};

export function displayTitle(fact: MemoryFact): string {
  return fact.title || fact.name.replaceAll("-", " ");
}

export function memoryFactKey(fact: MemoryFact): string {
  return fact.id || `${fact.scope}:${fact.name}`;
}

export function formatMemoryTime(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function freshnessLabel(value: string, t: Translator): string {
  switch (value) {
    case "fresh": return t("memory.freshness.fresh");
    case "current": return t("memory.freshness.current");
    case "stale": return t("memory.freshness.stale");
    default: return value;
  }
}

export function memoryMatches(fact: MemoryFact, normalizedQuery: string, typeFilter: string): boolean {
  if (typeFilter !== "all" && fact.type !== typeFilter) return false;
  if (!normalizedQuery) return true;
  return [displayTitle(fact), fact.name, fact.description, fact.type, fact.scope, fact.body]
    .join(" ")
    .toLowerCase()
    .includes(normalizedQuery);
}

export function archiveKey(fact: MemoryArchive): string {
  return `${fact.path || fact.name}:${fact.archivedAt || ""}`;
}

export function formatArchivedAt(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function uniqueLinks(body: string, names: Set<string>): LinkInfo[] {
  const links: LinkInfo[] = [];
  const seen = new Set<string>();
  const re = /\[\[([^\]]+)\]\]/g;
  let match: RegExpExecArray | null;
  while ((match = re.exec(body)) !== null) {
    const name = match[1].trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    links.push({ name, exists: names.has(name) });
  }
  return links;
}

export function memoryScopeLabel(scope: string, t: Translator): string {
  switch (scope) {
    case "project":
      return t("memory.scope.project");
    case "global":
      return t("memory.scope.global");
    case "user":
      return t("memory.scope.user");
    case "local":
      return t("memory.scope.local");
    case "ancestor":
      return t("memory.scope.ancestor");
    default:
      return scope;
  }
}

export function memoryTypeLabel(type: string, t: Translator): string {
  switch ((type || "").toLowerCase()) {
    case "project":
      return t("memory.type.project");
    case "user":
      return t("memory.type.user");
    case "feedback":
      return t("memory.type.feedback");
    case "reference":
      return t("memory.type.reference");
    default:
      return type || t("memory.type.other");
  }
}

export function memoryDocTitle(scope: string, t: Translator): string {
  switch (scope) {
    case "project":
      return t("memory.doc.projectTitle");
    case "user":
      return t("memory.doc.userTitle");
    case "local":
      return t("memory.doc.localTitle");
    case "ancestor":
      return t("memory.doc.ancestorTitle");
    default:
      return t("memory.doc.customTitle");
  }
}

export function memoryDocHint(scope: string, t: Translator): string {
  switch (scope) {
    case "project":
      return t("memory.doc.projectHint");
    case "user":
      return t("memory.doc.userHint");
    case "local":
      return t("memory.doc.localHint");
    case "ancestor":
      return t("memory.doc.ancestorHint");
    default:
      return t("memory.doc.customHint");
  }
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err || "Unknown error");
}

export function suggestionTotal(view: MemorySuggestionsView | null): number {
  return (view?.memories?.length ?? 0) + (view?.skills?.length ?? 0);
}

export function suggestionStamp(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
