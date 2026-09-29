// Footer-panel module: the memory/skill candidates the history scan produced, so
// they can be confirmed from the bottom band instead of the settings page. The
// scan reads local history only — no model call — so fetching it per session is
// free, and a session with nothing pending renders no section at all.

import { useCallback, useEffect, useState } from "react";
import { Check, RotateCw } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { MemorySuggestion, SkillSuggestion } from "../lib/types";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";

type Suggestions = { memories: MemorySuggestion[]; skills: SkillSuggestion[] };
type Candidate = { key: string; candidate: MemorySuggestion | SkillSuggestion; kind: "memory" | "skill"; label: string };

export function FooterMemorySuggestionsModule({ tabId }: FooterPanelModuleProps) {
  const t = useT();
  const [suggestions, setSuggestions] = useState<Suggestions | null>(null);
  const [accepted, setAccepted] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!tabId) return;
    let cancelled = false;
    setLoading(true);
    void Promise.resolve()
      .then(() => app.MemorySuggestionsForTab(tabId))
      .then((next) => {
        if (cancelled) return;
        setSuggestions({ memories: next?.memories ?? [], skills: next?.skills ?? [] });
        setAccepted({});
        setError("");
      })
      .catch((failure: unknown) => {
        if (cancelled) return;
        setError(failure instanceof Error ? failure.message : String(failure));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tabId, revision]);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);

  const accept = useCallback(
    async (candidate: MemorySuggestion | SkillSuggestion, kind: "memory" | "skill") => {
      if (!tabId) return;
      setBusy(true);
      try {
        if (kind === "memory") await app.AcceptMemorySuggestionForTab(tabId, candidate as MemorySuggestion);
        else await app.AcceptSkillSuggestionForTab(tabId, candidate as SkillSuggestion);
        setAccepted((current) => ({ ...current, [candidate.id]: true }));
      } catch (failure) {
        setError(failure instanceof Error ? failure.message : String(failure));
      } finally {
        setBusy(false);
      }
    },
    [tabId],
  );

  if (!tabId) return null;
  // Nothing pending (or no answer yet) means nothing to confirm: the section
  // stays away rather than holding an empty promise list.
  if (!suggestions || suggestions.memories.length + suggestions.skills.length === 0) return null;

  const candidates: Candidate[] = [
    ...suggestions.memories.map((candidate) => ({ key: `memory:${candidate.id}`, candidate, kind: "memory" as const, label: candidate.title || candidate.name })),
    ...suggestions.skills.map((candidate) => ({ key: `skill:${candidate.id}`, candidate, kind: "skill" as const, label: candidate.name })),
  ];

  return (
    <FooterPanelSection title="memory.suggestions">
      <div className="footer-suggest">
        <div className="footer-panel__bar">
          <span>{`${t("memory.memoryCandidates")} ${suggestions.memories.length} · ${t("memory.skillCandidates")} ${suggestions.skills.length}`}</span>
          <button
            type="button"
            className="footer-panel__refresh"
            title={t("memory.scanSuggestions")}
            aria-label={t("memory.scanSuggestions")}
            onClick={refresh}
          >
            <RotateCw className={loading ? "footer-panel__spin" : undefined} size={13} aria-hidden="true" />
          </button>
        </div>
        <ul className="footer-suggest__list">
          {candidates.map(({ key, candidate, kind, label }) => (
            <li className="footer-suggest__row" key={key} title={candidate.description}>
              <span className="footer-panel__badge">
                {kind === "memory" ? (candidate as MemorySuggestion).type : (candidate as SkillSuggestion).scope}
              </span>
              <span className="footer-suggest__title">{label}</span>
              <span className="footer-suggest__desc">{candidate.description}</span>
              {accepted[candidate.id] ? (
                <span className="footer-suggest__accepted">
                  <Check size={11} aria-hidden="true" />
                  {kind === "memory" ? t("memory.savedSuggestion") : t("memory.createdSkillSuggestion")}
                </span>
              ) : (
                <button
                  type="button"
                  className="footer-suggest__accept"
                  disabled={busy}
                  onClick={() => void accept(candidate, kind)}
                >
                  <Check size={11} aria-hidden="true" />
                  {kind === "memory" ? t("memory.saveAsMemory") : t("memory.createSkill")}
                </button>
              )}
            </li>
          ))}
        </ul>
        {error ? <p className="footer-panel__note footer-panel__note--error">{error}</p> : null}
      </div>
    </FooterPanelSection>
  );
}
