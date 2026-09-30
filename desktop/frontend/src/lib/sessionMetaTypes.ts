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

// SessionRecapEntry is one distilled note. It stays a candidate until a person
// accepts it; target is where it goes once accepted, id names it for a review
// action, and decision is the choice already recorded (absent when untouched).
export interface SessionRecapEntry {
  id: string;
  kind: "fact" | "root-cause" | "refuted" | "handoff" | string;
  body: string;
  evidence?: string;
  target: "memory" | "display" | string;
  decision?: "accept" | "reject" | string;
}

// SessionRecap is one read-only recap written when a session closes.
export interface SessionRecap {
  path: string;
  entries: SessionRecapEntry[];
  model: string;
  generatedAt: string; // RFC3339
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
}
