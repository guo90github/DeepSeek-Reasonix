import { useState } from "react";
import { useT } from "../lib/i18n";
import { AuditModal } from "./AuditModal";

// AuditInlineCard is the trigger for ONE reasoning block's audit: a styled
// button that opens AuditModal, which runs the audit and shows the full
// request → live process → conclusion chain in a centered modal. The run state
// lives inside the modal and is discarded when it closes (view-once lifecycle).
export function AuditInlineCard({ reasoning }: { reasoning: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);

  return (
    <>
      <button type="button" className="audit-btn" title={t("audit.runHint")} onClick={() => setOpen(true)}>
        {t("audit.run")}
      </button>
      {open && <AuditModal reasoning={reasoning} onClose={() => setOpen(false)} />}
    </>
  );
}
