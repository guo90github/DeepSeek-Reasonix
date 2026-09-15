import assert from "node:assert/strict";
import test from "node:test";
import type { WindowBounds } from "../shared/ipc.js";
import { WindowMover, restoredOriginUnderCursor, type WindowMovePort } from "./windowMove.js";

const bounds = (x: number, y: number, width: number, height: number, maximised = false): WindowBounds => ({ x, y, width, height, maximised });

class FakePort implements WindowMovePort {
  cursor = { x: 0, y: 0 };
  current = bounds(100, 100, 1200, 800);
  maximised = false;
  readonly moves: Array<{ x: number; y: number }> = [];
  unmaximiseCalls = 0;

  cursorPoint(): { x: number; y: number } { return this.cursor; }
  bounds(): WindowBounds { return this.current; }
  isMaximised(): boolean { return this.maximised; }
  unmaximise(): void {
    this.unmaximiseCalls += 1;
    this.maximised = false;
  }
  setPosition(x: number, y: number): void {
    this.moves.push({ x, y });
    this.current = bounds(x, y, this.current.width, this.current.height);
  }
}

class FakeClock {
  private tick: (() => void) | null = null;
  stops = 0;

  readonly schedule = (_ms: number, tick: () => void): (() => void) => {
    this.tick = tick;
    return () => {
      this.stops += 1;
      this.tick = null;
    };
  };

  advance(times = 1): void {
    for (let i = 0; i < times; i += 1) this.tick?.();
  }

  get scheduled(): boolean { return this.tick !== null; }
}

test("a restore keeps the grabbed point under the cursor", () => {
  const origin = restoredOriginUnderCursor({
    maximisedBounds: bounds(0, 0, 2560, 1440),
    restored: { width: 1280, height: 800 },
    cursor: { x: 2400, y: 30 },
    grab: { x: 1280, y: 20 },
  });
  assert.deepEqual(origin, { x: 2400 - 640, y: 30 - Math.round((20 / 1440) * 800) });
});

test("a grab outside the bar is clamped onto the restored frame", () => {
  const left = restoredOriginUnderCursor({
    maximisedBounds: bounds(0, 0, 2000, 1000),
    restored: { width: 1000, height: 700 },
    cursor: { x: 50, y: 50 },
    grab: { x: -40, y: 5000 },
  });
  assert.deepEqual(left, { x: 50, y: 50 - Math.round(0.5 * 700) });
  const right = restoredOriginUnderCursor({
    maximisedBounds: bounds(0, 0, 2000, 1000),
    restored: { width: 1000, height: 700 },
    cursor: { x: 10, y: 10 },
    grab: { x: 99999, y: 0 },
  });
  assert.deepEqual(right, { x: 10 - 1000, y: 10 });
});

test("a plain window follows the cursor by the delta it has moved", () => {
  const port = new FakePort();
  const clock = new FakeClock();
  const mover = new WindowMover(port, clock.schedule);
  port.cursor = { x: 500, y: 500 };
  mover.begin({ x: 300, y: 20 });
  assert.ok(mover.active);
  clock.advance();
  assert.deepEqual([...port.moves], [], "a press without movement does not reposition the window");

  port.cursor = { x: 530, y: 480 };
  clock.advance();
  assert.deepEqual([...port.moves], [{ x: 130, y: 80 }]);
  assert.equal(port.unmaximiseCalls, 0);
});

test("a maximised window is restored under the cursor, then follows", () => {
  const port = new FakePort();
  const clock = new FakeClock();
  const mover = new WindowMover(port, clock.schedule);
  port.maximised = true;
  port.current = bounds(-2560, 0, 2560, 1440, true);
  port.cursor = { x: -2500, y: 30 };

  mover.begin({ x: 1280, y: 24 });
  assert.equal(port.unmaximiseCalls, 1);
  clock.advance(3);
  assert.deepEqual([...port.moves], [], "nothing moves while the restore has not landed");

  port.current = bounds(-2000, 200, 1280, 800);
  port.cursor = { x: -2400, y: 40 };
  clock.advance();
  assert.deepEqual([...port.moves], [{ x: -2400 - 640, y: 40 - Math.round((24 / 1440) * 800) }], "the restore places the grab point under the cursor");

  port.cursor = { x: -2390, y: 30 };
  clock.advance();
  assert.deepEqual(port.moves[1], { x: port.moves[0].x + 10, y: port.moves[0].y - 10 }, "the follow continues from the restored origin");
});

test("a restore that never lands still starts following the cursor", () => {
  const port = new FakePort();
  const clock = new FakeClock();
  const mover = new WindowMover(port, clock.schedule);
  port.maximised = true;
  port.current = bounds(0, 0, 2560, 1440, true);
  port.cursor = { x: 100, y: 100 };

  mover.begin({ x: 10, y: 10 });
  clock.advance(40);
  assert.equal(port.moves.length, 1, "the settle cap releases the anchor");
  port.cursor = { x: 110, y: 100 };
  clock.advance();
  assert.equal(port.moves.length, 2);
  assert.equal(port.moves[1].x, port.moves[0].x + 10);
});

test("end stops the follow and a second begin replaces it", () => {
  const port = new FakePort();
  const clock = new FakeClock();
  const mover = new WindowMover(port, clock.schedule);
  mover.begin({ x: 10, y: 10 });
  mover.begin({ x: 10, y: 10 });
  assert.equal(clock.stops, 1, "the previous follow is stopped, not leaked");

  mover.end();
  assert.equal(mover.active, false);
  assert.equal(clock.scheduled, false);
  clock.advance(5);
  assert.deepEqual([...port.moves], [], "a stopped mover never repositions the window");
  mover.end();
  assert.equal(clock.stops, 2);
});
