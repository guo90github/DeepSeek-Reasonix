// The split layout's footer-panel module registry — the single plug point for
// the bottom-band card. Append an entry and FooterPanel renders whatever that
// module returns; nothing in the shell or the card changes.
//
// Order is reading order: this session's own edits, then the workspace's git
// state, then history, then the memory family.

import { FooterChangedFilesModule } from "./FooterChangedFilesModule";
import { FooterGitHistoryModule } from "./FooterGitHistoryModule";
import { FooterGitUncommittedModule } from "./FooterGitUncommittedModule";
import { FooterMemoryModule } from "./FooterMemoryModule";
import { FooterMemorySuggestionsModule } from "./FooterMemorySuggestionsModule";
import { FooterRecallModule } from "./FooterRecallModule";
import type { FooterPanelModule } from "./FooterPanel";

export const FOOTER_PANEL_MODULES: readonly FooterPanelModule[] = [
  {
    id: "session-changes",
    render: (props) => <FooterChangedFilesModule {...props} />,
  },
  {
    id: "git-uncommitted",
    render: (props) => <FooterGitUncommittedModule {...props} />,
  },
  {
    id: "git-history",
    render: (props) => <FooterGitHistoryModule {...props} />,
  },
  {
    id: "memory",
    render: (props) => <FooterMemoryModule {...props} />,
  },
  {
    id: "memory-suggestions",
    render: (props) => <FooterMemorySuggestionsModule {...props} />,
  },
  {
    id: "recall-record",
    render: (props) => <FooterRecallModule {...props} />,
  },
];
