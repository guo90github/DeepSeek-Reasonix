import { useState, type MouseEvent, type ReactNode } from "react";

// One row shape for every list on the recap page: badge, summary, actions, then an
// optional expanded part. The fixed grid is the point — inline flow is what made the
// same row wrap on some widths and not others. The summary is clipped in CSS only,
// so copying and searching still see the full text, and clicking (not just hovering)
// opens the detail, which keeps it reachable by keyboard and on touch.
type Props = {
  badge: ReactNode;
  text: string;
  leading?: ReactNode;
  actions?: ReactNode;
  detail?: ReactNode;
  editor?: ReactNode;
  tone?: "warn";
};

const muted = { opacity: 0.72, fontSize: 12 } as const;

export function RecapRow({ badge, text, leading, actions, detail, editor, tone }: Props) {
  const [open, setOpen] = useState(false);
  const expanded = open || editor !== undefined;
  // Anywhere on the row opens it, except on the row's own controls: aiming at the
  // text is a needless precision requirement, and hovering anywhere should say what
  // the clipped summary holds.
  const toggleOnHead = (event: MouseEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement | null;
    if (target !== null && target.closest("button, input, a, textarea, select") !== null) return;
    setOpen((current) => !current);
  };
  return (
    <div data-recap-row="" style={{ display: "flex", flexDirection: "column", gap: 4, color: tone === "warn" ? "var(--warn, inherit)" : undefined }}>
      <div onClick={toggleOnHead} title={text}
        style={{ display: "grid", gridTemplateColumns: "auto auto minmax(0, 1fr) auto", gap: 6, alignItems: "baseline" }}>
        {leading}
        <span style={{ ...muted, whiteSpace: "nowrap" }}>{badge}</span>
        {/* A span, not a button: the summary is not an action, and the row's own
            buttons should stay countable as actions. Keyboard still opens it. */}
        <span role="button" tabIndex={0}
          onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); setOpen((current) => !current); } }}
          style={{ cursor: "pointer", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: expanded ? "normal" : "nowrap" }}>
          {text}
        </span>
        <span style={{ display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap", justifyContent: "flex-end" }}>{actions}</span>
      </div>
      {/* The cited ground is never hidden — it just stops competing with the summary
          for the same line, which is what made rows wrap differently at each width. */}
      {detail !== undefined && (
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap", ...muted }}>{detail}</div>
      )}
      {editor}
    </div>
  );
}

export const recapRowMuted = muted;
