import type { WindowBounds } from "../shared/ipc.js";

export interface WindowMovePort {
  cursorPoint(): { x: number; y: number };
  bounds(): WindowBounds;
  isMaximised(): boolean;
  unmaximise(): void;
  setPosition(x: number, y: number): void;
}

export type WindowMoveSchedule = (ms: number, tick: () => void) => () => void;

const FOLLOW_INTERVAL_MS = 16;
const RESTORE_SETTLE_TICKS = 40;

const clamp = (value: number, min: number, max: number): number => Math.min(max, Math.max(min, value));

/** The origin that keeps the cursor at the relative place it held on the bar. */
export function restoredOriginUnderCursor(input: {
  maximisedBounds: WindowBounds;
  restored: { width: number; height: number };
  cursor: { x: number; y: number };
  grab: { x: number; y: number };
}): { x: number; y: number } {
  const xFraction = clamp(input.grab.x / Math.max(1, input.maximisedBounds.width), 0, 1);
  const yFraction = clamp(input.grab.y / Math.max(1, input.maximisedBounds.height), 0, 0.5);
  return {
    x: Math.round(input.cursor.x - xFraction * input.restored.width),
    y: Math.round(input.cursor.y - yFraction * input.restored.height),
  };
}

/**
 * Cursor-driven window move for the shell's drag surface. A maximised frameless
 * window gets no native caption drag on Windows and the press never reaches the
 * renderer either, so the page asks for the move and the cursor is polled here
 * rather than riding renderer mouse events.
 */
export class WindowMover {
  private stopFollow: (() => void) | null = null;
  private origin = { x: 0, y: 0 };
  private anchor = { x: 0, y: 0 };
  private pending: { maximisedBounds: WindowBounds; grab: { x: number; y: number } } | null = null;

  constructor(private readonly port: WindowMovePort, private readonly schedule: WindowMoveSchedule) {}

  get active(): boolean {
    return this.stopFollow !== null;
  }

  begin(grab: { x: number; y: number }): void {
    this.end();
    this.anchor = this.port.cursorPoint();
    if (this.port.isMaximised()) {
      this.pending = { maximisedBounds: this.port.bounds(), grab };
      this.port.unmaximise();
    } else {
      const bounds = this.port.bounds();
      this.origin = { x: bounds.x, y: bounds.y };
    }
    let ticks = 0;
    this.stopFollow = this.schedule(FOLLOW_INTERVAL_MS, () => {
      ticks += 1;
      const cursor = this.port.cursorPoint();
      const pending = this.pending;
      if (pending) {
        // Electron restores the geometry asynchronously; anchor once it lands.
        const restored = this.port.bounds();
        const landed = restored.width !== pending.maximisedBounds.width || restored.height !== pending.maximisedBounds.height;
        if (!landed && ticks < RESTORE_SETTLE_TICKS) return;
        this.pending = null;
        this.origin = restoredOriginUnderCursor({ maximisedBounds: pending.maximisedBounds, restored, cursor, grab: pending.grab });
        this.anchor = cursor;
        this.port.setPosition(this.origin.x, this.origin.y);
        return;
      }
      if (cursor.x === this.anchor.x && cursor.y === this.anchor.y) return;
      this.port.setPosition(this.origin.x + cursor.x - this.anchor.x, this.origin.y + cursor.y - this.anchor.y);
    });
  }

  end(): void {
    this.stopFollow?.();
    this.stopFollow = null;
    this.pending = null;
  }
}
