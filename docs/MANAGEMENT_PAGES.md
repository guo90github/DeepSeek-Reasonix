# Full-window management pages

Trash, Session recap, Automation and Settings retain independent entries and share `ManagementPageShell`. They fill the application window without changing native fullscreen, maximize or close behavior. Each feature loads lazily; a failed chunk can be retried or exited.

## Navigation and workspace

`useAppNavigationStore` owns workspace/settings/trash/recap/automation navigation, the last settings category, focus restoration and the single automation-conversation return target. The legacy providers category resolves to models. Transient menus and dialogs remain independent overlays.

The workspace stays mounted at its existing dimensions and becomes inert while a management page is active. Returning does not reactivate the current conversation, reload history, or introduce another transcript scroll writer. Background work continues. Entry focuses Back to workspace; exit restores the invoking entry. Escape closes child menus/dialogs, while the close-tab shortcut returns to the workspace. Hidden workspace shortcuts are suspended; command palette, settings and explicit conversation navigation remain available.

Opening an automation's linked conversation uses the existing guarded navigation queue. Success exposes Back to automation; failure keeps the editor visible. A newer navigation invalidates earlier results. Deliberate project/conversation changes clear the temporary return target.

## Trash

The wide layout places search, scope/date filters and grouped conversations on the left, with a read-only preview on the right. System recovery copies remain separately collapsed. Normal history keeps its existing dialog and shares the list/preview implementation.

Selection uses stable paths and advances to the nearest remaining row after a mutation. Preview request generations prevent late responses from replacing a newer selection. Loading failures are distinct from empty results. Restore and purge have explicit asynchronous outcomes.

Permanent deletion requires confirmation with initial focus on Cancel. Empty Trash snapshots every ordinary deleted conversation, including filtered-out rows, and excludes system recovery copies. The batch runs sequentially, continues after individual failures, reports totals and offers an explicit failed-only retry. A successful mutation followed by a failed refresh is reported separately, so successful destructive operations are not resubmitted.

## Session recap

Session recap keeps its own entry and is deliberately not merged into Trash, which owns a session's restore and purge. The page is read-only: a recap is the four-element document written when a session closes, so it lists closed sessions only, and its subtitle states that session-level deletion and restore live in Trash. Rows come from the host's `ListSessionRecaps` newest first; sessions that left the visible set are dropped host-side, so the page shows exactly what retrieval still sees. Generating a recap never blocks a close: the lane is asynchronous and its own provider call.


### How the page renders, and its batch actions

- **Every note is one fixed three-column row**: `checkbox / badge / summary / actions`. Actions **always sit in the same right-hand column**, so a row no longer wraps on one width and not another; evidence, pointers, proposed tier and cross-project counts moved to a muted second line. The summary is clipped **in CSS only** (copying and searching still see the full text), hovering anywhere on the row shows the whole text, and clicking **anywhere on the row** (or Enter/Space) expands it.
- **Notes of one topic share one action row**: grouping happens only **within a kind** (a fact and a handoff stay apart even on one topic — they land in different places), and the rule is ASCII words plus adjacent CJK pairs, requiring two real identifiers (length >= 4; `t1`, `id`, `sql` do not count) or a shorter note nearly contained in the other. The rule is **presentation only**: every note stays readable and acceptable on its own, and the checkboxes decide which of them "write to memory" covers. A group past six notes shows six with the rest behind "N more".
- **Session cards expand only the newest one by default** (following sort and search); the title is the fold control, the toolbar has expand-all/collapse-all, and the header shows the note count. A **failed attempt's line never folds away**: it is a state, and it has to stay visible.
- **Rows are split by rule version**: every record carries the version that produced it, and sessions older than the running rules are listed under "N recaps were produced by an older rule set", regenerable one at a time or in bulk.

### The two buttons go through a preview

"Write to memory" and "draft a skill" now each ask their own prompt for a model-written draft and show it for **confirmation** before anything is stored (one extra call per press, accepted by the user, with no token ceiling of ours):

- **Write to memory**: the panel shows each note and the model's rewrite beside it, one editable box per note; confirming stores **exactly the text shown** (it arrives through the same "edited body" path).
- **Draft a skill**: the panel shows the whole markdown (monospaced, editable); confirming writes **the reviewed text as it stands**, adding only the frontmatter (`name` / `description` / `invocation: manual`). The verbatim composition stays as the way out: "use the verbatim composition instead" in the panel, and the fallback whenever no model answers.
- The panel always names its **provenance** (prompt version and model), states the **reason** when generation failed or returned nothing, and keeps a no-model path — a failed generation never leaves the button unusable.
- "Don't save", "edit and store" and "keep as an unfinished item" are unchanged (editing a note by hand still bypasses the model).

### Drafting a skill (playbook)

Both the note row and the **topic group header** offer "draft a skill". One topic composes **one** playbook: every note is kept **verbatim** with its kind, its own pointers (`Where to check`) and its evidence line, and the file opens by saying it came from N notes on one topic with nothing rewritten or inferred — reliability comes from citable ground rather than from guessing a procedure out of several notes. **Different topics are different files**, so a batch is honestly one file or several. Drafts land in this project's `.reasonix/skills/recap-*/SKILL.md`, declare `invocation: manual`, **never overwrite an existing file**, and are only ever person-triggered; the host refuses anything that is not a procedure (facts, unfinished items).

### The model and the prompt behind a recap

- **The model is configured independently**: the model the session itself recorded first, then the `agent.session_recap_model` setting.
- **The prompt is built-in rules plus an optional override**: the built-in rule set is versioned `recap-v9`; dropping `recap-prompt.md` into the **local state directory** (the one holding the recap projection) replaces it, the file is sent **verbatim**, and records then carry `recap-v9+<8 hex>`. An empty, oversized (past 32 KiB) or unreadable file **falls back to the built-in rules and says why** — half a prompt would silently lower the quality of everything the lane writes. This prompt **never enters the main session's system-prompt prefix**, so changing it leaves that prompt cache untouched.

## Automation drafts

`useAutomationDraftStore` keeps per-ID baselines, editable values, frequency choice, detail tab, conflicts and operation versions in memory for the application run. Switching tasks, filters, pages, detail visibility and linked conversations preserves drafts. New unsaved tasks remain discoverable; changed existing tasks show an Unsaved badge. Reloading the webview or exiting the process clears drafts; minimize/tray hiding does not.

Only Save commits scheduling configuration. Failed saves preserve input. A pending operation locks its task without preventing navigation to another task; completion never steals selection. Discard restores the latest baseline or removes an unsaved new task. Run now uses saved configuration, and enable/pause updates only enabled. Confirmed deletion stops future scheduling while retaining existing conversations.

The existing serial mutation queue and revision/etag checks remain. Engine-owned run fields merge without losing edits; disjoint external configuration changes merge; conflicting fields block normal saving and offer Reload latest or Save as a new paused task. Externally deleted tasks retain recoverable drafts without resurrecting the original ID. New-task ID collisions allocate another ID. Results are bound to task identity and operation version.

## Layout and compatibility

The shared titlebar reserves 44px on macOS and 48px on Windows, with existing 46px Windows controls. Stable chrome is draggable; controls/content are not. At 960px and above, lists and details use two columns (minimum 280px and 420px). Narrow windows switch between list and detail without losing input. Trash defaults to a 34% list; Automation restores a valid width preference or uses 40%. Content scrolls independently and primary actions remain visible.

New copy in `managementLocale.ts` covers Simplified Chinese, Traditional Chinese and English. Existing theme tokens provide light/dark styling. No configuration migration or model/prompt/tool/cache-protocol changes are required.

## Verification (2026-09-05)

Deterministic tests cover navigation generations, draft merging/conflicts/deletion, switching tasks during save, failures, keyboard isolation, preview races, recovery-copy exclusion, partial deletion and post-success refresh failure. All 264 frontend suites passed, including heartbeat/history/settings; the separate transcript regression command also passed. Type checking and Hooks/CSS/layer/theme/WAAPI/single-scroll-writer checks run with the production build.

Windows browser preview was exercised at 1280×900 and 800×800, including model settings, trash confirmation/cancellation, and draft/detail retention. No real conversations were purged and no automation task was executed. An isolated macOS native build was checked for page entry, Escape, Command-W return, minimize/restore and window zoom. Windows ARM64 built and launched with isolated data in Win11, displaying onboarding. Native Windows interactions and 100%/125%/150% DPI checks remain unverified; browser preview is not a substitute.

The initial payload measures approximately 465.4 KiB gzip JavaScript and 2480.9 KiB raw JavaScript/CSS, versus 464.7/2481.7 KiB on the integrated main-v2 base. The gzip ceiling is 465.5 KiB; the upstream raw ceiling remains 2481.7 KiB. The Traditional Chinese chunk rounds up to a 61.7 KiB ceiling. Feature bodies remain lazy and other budgets remain enforced.

The integrated titlebar dispatch was checked in an isolated macOS native build: double-click maximizes and a second double-click restores the original window size. Unsaved credential discovery uses a transient capability resolver and cannot pollute the saved capability cache.
