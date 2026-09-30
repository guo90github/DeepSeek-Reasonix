// SessionMeta is one saved session for the history panel.
export interface SessionMeta {
  path: string;
  preview: string;
  title?: string; // user-chosen name; falls back to preview when empty
  turns: number;
  turnsState?: "unknown" | "valid" | "corrupt" | string;
  createdAt: number; // unix milliseconds
  lastActivityAt: number; // unix milliseconds
  modTime: number; // compatibility alias for lastActivityAt
  deletedAt?: number; // unix milliseconds, present for trashed sessions
  current: boolean;
  open: boolean;
  scope?: string; // "project" | "global"; empty for legacy → treated as "global"
  workspaceRoot?: string;
  topicId?: string;
  topicTitle?: string;
  kind?: "session" | "channel" | string;
  channel?: string;
  channelLabel?: string;
  remoteId?: string;
  chatType?: string;
  userId?: string;
  threadId?: string;
  sessionSource?: string;
  recovered?: boolean; // created by conflict recovery, including a continued branch
  recoveryCopy?: boolean; // actual branch content is unchanged and covered by its parent
  recoveryGroupId?: string;
  recoveryRole?: string; // normal|covered_copy|adopted|diverged
  recoveryCanonical?: boolean;
  versionKind?: "normal" | "recovery" | "subagent" | string;
  versionState?: "active" | "pending" | "resolved" | "trashed" | string;
  parentVersionId?: string;
}

// SessionRecapRef is one place a note says it can be checked at: a path, a
// command, a test, a turn range, or a config key.
export interface SessionRecapRef {
  kind: string;
  value: string;
  detail?: string;
}

// SessionRecapEntry is one distilled note. It stays a candidate until a person
// accepts it; target is where it goes once accepted, id names it for a review
// action, and decision is the choice already recorded (absent when untouched).
// refs are the places it can be checked at, and scope is the deposition tier the
// model proposed for it — shown, never applied on its own.
export interface SessionRecapEntry {
  id: string;
  kind: "fact" | "root-cause" | "refuted" | "handoff" | string;
  body: string;
  evidence?: string;
  target: "memory" | "display" | string;
  decision?: "accept" | "reject" | string;
  refs?: SessionRecapRef[];
  scope?: "project" | "base" | "generic" | string;
  scopeReason?: string;
  // observedIn names the other projects that reached a conclusion like this one:
  // the evidence a proposed tier rests on, filled in only when it exists.
  observedIn?: string[];
}

// SessionRecapPending is a failed generation attempt. A failure stores no
// record, so this is the only thing that can tell a reader the recap they expect
// was attempted and refused.
export interface SessionRecapPending {
  attempts: number;
  reason: string;
  updatedAt: string; // RFC3339
}

// SessionRecap is one read-only recap written when a session closes. State says
// which of the three things a row is — a stored recap, a failed attempt still
// being retried, or a session that produced nothing — so the page never has to
// guess between "no reusable notes" and "the attempt failed".
export interface SessionRecap {
  path: string;
  state?: "stored" | "pending" | "empty" | string;
  entries: SessionRecapEntry[];
  model: string;
  generatedAt: string; // RFC3339, empty when the last attempt failed
  pending?: SessionRecapPending;
}

// RecapOpenItem is one unfinished item kept from a handoff note. It belongs to a
// project and stays listed until someone marks it handled.
export interface RecapOpenItem {
  id: string;
  body: string;
  evidence?: string;
  from?: string;
  openedAt: string; // RFC3339
  closed: boolean;
  stale?: boolean;
  ageDays?: number;
}
