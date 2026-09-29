// The split layout's bottom-band card: one card holding pluggable modules. A
// module owns its own section through FooterPanelSection and may render nothing
// at all when it does not apply to the current workspace — an inapplicable
// module must not leave a header that can never fill. The module list lives in
// footerPanelModules.tsx.

import { Fragment, useCallback, useState, type ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { useT, type DictKey } from "../lib/i18n";

export type FooterPanelModuleProps = {
  tabId?: string;
  workspaceScopeKey: string;
};

export type FooterPanelModule = {
  id: string;
  render: (props: FooterPanelModuleProps) => ReactNode;
};

/** One module's section: the card supplies the chrome, the module the content. */
export function FooterPanelSection({
  title,
  defaultOpen = true,
  children,
}: {
  title: DictKey;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const t = useT();
  const [open, setOpen] = useState(defaultOpen);
  const toggle = useCallback(() => setOpen((current) => !current), []);
  return (
    <section className="footer-panel__section">
      <button
        type="button"
        className="footer-panel__section-head"
        aria-expanded={open}
        onClick={toggle}
      >
        <ChevronDown
          className={`footer-panel__chevron${open ? "" : " footer-panel__chevron--closed"}`}
          size={14}
          aria-hidden="true"
        />
        <span className="footer-panel__section-title">{t(title)}</span>
      </button>
      {open ? <div className="footer-panel__section-body">{children}</div> : null}
    </section>
  );
}

export function FooterPanel({
  modules,
  context,
}: {
  modules: readonly FooterPanelModule[];
  context: FooterPanelModuleProps;
}) {
  const t = useT();
  return (
    <section className="footer-panel" aria-label={t("footerPanel.title")}>
      <header className="footer-panel__head">{t("footerPanel.title")}</header>
      <div className="footer-panel__stack">
        {modules.map((module) => <Fragment key={module.id}>{module.render(context)}</Fragment>)}
      </div>
    </section>
  );
}
