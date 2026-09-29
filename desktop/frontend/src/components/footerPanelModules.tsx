// The split layout's footer-panel module registry — the single plug point for
// the bottom-band card. Append an entry and FooterPanel renders whatever that
// module returns; nothing in the shell or the card changes.

import { FooterChangedFilesModule } from "./FooterChangedFilesModule";
import type { FooterPanelModule } from "./FooterPanel";

export const FOOTER_PANEL_MODULES: readonly FooterPanelModule[] = [
  {
    id: "changed-files",
    render: (props) => <FooterChangedFilesModule {...props} />,
  },
];
