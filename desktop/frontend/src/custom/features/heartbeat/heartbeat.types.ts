// Heartbeat task types — mirrors desktop/heartbeat.go.

export interface HeartbeatRun {
  at: number;      // unix millis execution time
  topicId: string; // topic used/created by this run
}

export interface HeartbeatTask {
  id: string;
  title: string;
  prompt: string;
  goal?: string;      // unattended Goal contract; empty = a plain scheduled prompt
  interval: string;   // e.g. "5m", "1h", "30s"
  enabled: boolean;
  scope?: string;      // "global" or "project"
  workspaceRoot?: string;
  topicId?: string;
  lastRunAt?: number;  // unix millis, moved only by a real run
  lastAttemptAt?: number; // unix millis, a tick spent without running (Goal hold / no topic)
  lastHold?: string;      // why the last tick left it alone (host-computed)
  lastHoldAt?: number;    // unix millis of that tick
  newConversationEachRun?: boolean; // true = create new topic each run
  runHistory?: HeartbeatRun[];      // recent executions (oldest first)
  createdAt?: number;
  approvalMode?: "ask" | "auto" | "yolo"; // empty defaults to "yolo"
  timeWindowStart?: string; // "HH:MM" — interval tasks only run after this time
  timeWindowEnd?: string;   // "HH:MM" — interval tasks only run before this time
  notifyChannels?: boolean; // true = push to bot channels; false/nil = skip
}
