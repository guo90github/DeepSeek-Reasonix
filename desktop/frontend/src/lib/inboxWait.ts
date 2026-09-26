// Split of the queue wait into the parts a label needs. The wait only answers
// "how long" — why it is not running yet is the gate's and the host's own words.
export type InboxWaitParts = {
  minutes: number;
  seconds: number;
};

// Returns null when there is nothing honest to show: no wait was reported, or the
// item is not waiting long enough for a whole second to have passed.
export function inboxWaitParts(waitMs?: number): InboxWaitParts | null {
  if (typeof waitMs !== "number" || !Number.isFinite(waitMs) || waitMs < 1000) return null;
  const total = Math.floor(waitMs / 1000);
  return { minutes: Math.floor(total / 60), seconds: total % 60 };
}
