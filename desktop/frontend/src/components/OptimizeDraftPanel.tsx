import { useCallback, useRef, useState } from "react";
import { Check, Pencil, RefreshCw, Sparkles, Trash2, X } from "lucide-react";
import { ANCHORED_POPOVER_CLOSE_MS, AnchoredPopover } from "./AnchoredPopover";
import { InlineDiff } from "./InlineDiff";
import { Tooltip } from "./Tooltip";
import { useI18n, type Translator } from "../lib/i18n";
import type { DictKey } from "../locales/en";

export type OptimizeDirection = "all" | "professional" | "concise" | "expand" | "contextual";

const DIRECTIONS: { id: OptimizeDirection; labelKey: DictKey; descKey: DictKey }[] = [
  { id: "all", labelKey: "composer.optimizeAll", descKey: "composer.optimizeAllDesc" },
  { id: "professional", labelKey: "composer.optimizeProfessional", descKey: "composer.optimizeProfessionalDesc" },
  { id: "concise", labelKey: "composer.optimizeConcise", descKey: "composer.optimizeConciseDesc" },
  { id: "expand", labelKey: "composer.optimizeExpand", descKey: "composer.optimizeExpandDesc" },
  { id: "contextual", labelKey: "composer.optimizeContextual", descKey: "composer.optimizeContextualDesc" },
];

export type OptimizePreview = { original: string; rewritten: string };

function directionLabel(t: Translator, id: OptimizeDirection): string {
  const entry = DIRECTIONS.find((d) => d.id === id);
  return entry ? t(entry.labelKey) : t("composer.optimizeAll");
}

// OptimizeDraftTrigger is the composer input-row button that opens the direction
// menu and starts an optimize call.
export function OptimizeDraftTrigger({
  disabled,
  optimizing,
  onSelect,
}: {
  disabled: boolean;
  optimizing: boolean;
  onSelect: (direction: OptimizeDirection) => void;
}) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [closing, setClosing] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);

  const close = useCallback(() => {
    setOpen(false);
    setClosing(true);
    window.setTimeout(() => setClosing(false), ANCHORED_POPOVER_CLOSE_MS);
  }, []);

  const choose = (direction: OptimizeDirection) => {
    close();
    onSelect(direction);
  };

  return (
    <>
      <Tooltip label={t("composer.optimize")} disabled={open}>
        <button
          ref={anchorRef}
          type="button"
          className={`composer__btn composer__btn--optimize${open ? " composer__btn--optimize-open" : ""}`}
          onClick={() => (open ? close() : setOpen(true))}
          disabled={disabled}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label={t("composer.optimize")}
        >
          {optimizing ? <RefreshCw size={16} className="composer__btn--spinning" /> : <Sparkles size={16} />}
        </button>
      </Tooltip>
      <AnchoredPopover
        open={open && !disabled}
        closing={closing}
        anchorRef={anchorRef}
        onClose={close}
        className="composer-access-menu composer-optimize-menu"
        align="end"
      >
        <div className="composer-access-menu__section" role="menu" aria-label={t("composer.optimizeMenuTitle")}>
          <div className="composer-optimize-menu__title">{t("composer.optimizeMenuTitle")}</div>
          {DIRECTIONS.map(({ id, labelKey, descKey }) => (
            <button
              key={id}
              type="button"
              role="menuitem"
              className="composer-access-menu__item composer-optimize-menu__item"
              onClick={() => choose(id)}
              disabled={optimizing}
            >
              <Sparkles size={16} aria-hidden="true" />
              <span className="composer-access-menu__copy">
                <span className="composer-access-menu__title">{t(labelKey)}</span>
                <span className="composer-access-menu__desc">{t(descKey)}</span>
              </span>
            </button>
          ))}
        </div>
      </AnchoredPopover>
    </>
  );
}

// OptimizeDraftPreview shows the optimize result above the composer: a diff of
// the original draft vs the rewrite, with apply / discard / re-optimize / edit.
export function OptimizeDraftPreview({
  loading,
  error,
  preview,
  direction,
  onApply,
  onDiscard,
  onRetry,
}: {
  loading: boolean;
  error: string | null;
  preview: OptimizePreview | null;
  direction: OptimizeDirection | null;
  onApply: (rewritten: string) => void;
  onDiscard: () => void;
  onRetry: () => void;
}) {
  const { t } = useI18n();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string>("");

  if (loading) {
    return (
      <div className="composer-optimize-card composer-optimize-card--status" role="status">
        <RefreshCw size={14} className="composer__btn--spinning" aria-hidden="true" />
        <span>{t("composer.optimizeLoading")}</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="composer-optimize-card composer-optimize-card--status composer-optimize-card--error" role="alert">
        <X size={14} aria-hidden="true" />
        <span>{error}</span>
        <button type="button" className="composer-optimize-card__close" onClick={onDiscard} aria-label={t("composer.optimizeDiscard")}>
          <Trash2 size={14} />
        </button>
      </div>
    );
  }

  if (!preview) return null;
  if (!preview.rewritten.trim()) {
    return (
      <div className="composer-optimize-card composer-optimize-card--status" role="status">
        <Check size={14} aria-hidden="true" />
        <span>{t("composer.optimizeUnchanged")}</span>
        <button type="button" className="composer-optimize-card__close" onClick={onDiscard} aria-label={t("composer.optimizeDiscard")}>
          <Trash2 size={14} />
        </button>
      </div>
    );
  }

  return (
    <div className="composer-optimize-card">
      <header className="composer-optimize-card__head">
        <Sparkles size={14} aria-hidden="true" />
        <span className="composer-optimize-card__title">
          {direction ? directionLabel(t, direction) : t("composer.optimizeDiffLabel")}
        </span>
        <span className="composer-optimize-card__spacer" />
        <button type="button" className="composer-optimize-card__head-btn" onClick={onRetry} title={t("composer.optimizeRetry")}>
          <RefreshCw size={13} />
          <span>{t("composer.optimizeRetry")}</span>
        </button>
        <button type="button" className="composer-optimize-card__close" onClick={onDiscard} aria-label={t("composer.optimizeDiscard")}>
          <Trash2 size={14} />
        </button>
      </header>

      {editing ? (
        <textarea
          className="composer-optimize-card__editor"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          aria-label={t("composer.optimizeEdit")}
        />
      ) : (
        <div className="composer-optimize-card__diff">
          <InlineDiff before={preview.original} after={preview.rewritten} filename={t("composer.optimizeDiffLabel")} />
        </div>
      )}

      <footer className="composer-optimize-card__actions">
        {editing ? (
          <>
            <button type="button" className="composer-optimize-card__btn composer-optimize-card__btn--primary" onClick={() => onApply(draft)}>
              <Check size={14} />
              <span>{t("composer.optimizeSaveEdit")}</span>
            </button>
            <button type="button" className="composer-optimize-card__btn" onClick={() => setEditing(false)}>
              <X size={14} />
              <span>{t("composer.optimizeCancelEdit")}</span>
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              className="composer-optimize-card__btn composer-optimize-card__btn--primary"
              onClick={() => onApply(preview.rewritten)}
            >
              <Check size={14} />
              <span>{t("composer.optimizeApply")}</span>
            </button>
            <button
              type="button"
              className="composer-optimize-card__btn"
              onClick={() => {
                setDraft(preview.rewritten);
                setEditing(true);
              }}
            >
              <Pencil size={14} />
              <span>{t("composer.optimizeEdit")}</span>
            </button>
            <button type="button" className="composer-optimize-card__btn" onClick={onDiscard}>
              <Trash2 size={14} />
              <span>{t("composer.optimizeDiscard")}</span>
            </button>
          </>
        )}
      </footer>
    </div>
  );
}
