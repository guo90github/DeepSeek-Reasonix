// Persistent jitter logger: watches both panes at 40ms; on a st jump ≥40px
// (fold-collapse or chase) OR st-reversal burst, appends an event line to a
// log FILE that survives across sessions. Runs until killed. Usage:
// node __persist-logger.cjs <outfile>
const { createRequire } = require("module");
const req = createRequire("C:/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend/package.json");
const { chromium } = req("playwright");
const fs = require("fs");

const OUT = process.argv[2] || "C:/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend/scripts/jitter-log.txt";

(async () => {
  const b = await chromium.connectOverCDP("http://127.0.0.1:9222");
  const page = b.contexts()[0].pages()[0];
  const t0 = Date.now();
  const st = {
    conversation: { last: null, dir: 0, revAt: 0, revs: 0 },
    process: { last: null, dir: 0, revAt: 0, revs: 0 },
  };
  const log = (line) => fs.appendFileSync(OUT, `[${new Date().toISOString().slice(11, 19)}] ${line}\n`);
  log("=== logger start ===");
  while (true) {
    const s = await page.evaluate(() => {
      const g = (sel) => {
        const el = document.querySelector(sel);
        if (!el) return null;
        return { st: Math.round(el.scrollTop), sh: el.scrollHeight };
      };
      return { c: g(".conversation-pane"), p: g(".process-pane") };
    });
    const now = Date.now();
    for (const [k, name] of [["c", "conversation"], ["p", "process"]]) {
      const v = s[k];
      const t = st[name];
      if (!v) continue;
      if (t.last !== null) {
        const d = v.st - t.last;
        const abs = Math.abs(d);
        if (abs >= 40) {
          log(`${name} ST-JUMP ${d > 0 ? "+" : ""}${d}px (st ${t.last}->${v.st}, sh ${v.sh})`);
        } else if (abs >= 4) {
          const dir = d > 0 ? 1 : -1;
          if (t.dir !== 0 && dir !== t.dir) {
            if (now - t.revAt < 700) {
              t.revs += 1;
              if (t.revs >= 2) log(`${name} JITTER (${t.revs} reversals, st ${t.last}->${v.st}, sh ${v.sh})`);
            } else t.revs = 1;
            t.revAt = now;
          } else t.revs = 0;
          t.dir = dir;
        }
      }
      t.last = v.st;
    }
    await page.waitForTimeout(40);
  }
})().catch((e) => { console.error("logger died:", e.message); process.exit(1); });
