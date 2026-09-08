// v47 jitter/storm verifier: samples both panes at ~30ms for N seconds,
// reports sh oscillation (max-min), scrollTop direction reversals, and write
// count/rate. A storm = sustained ≥4px sh oscillation with writes ≥5Hz.
// Self-contained; deletes its sink on exit.
const { createRequire } = require("module");
const req = createRequire("C:/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend/package.json");
const { chromium } = req("playwright");

const MAX_MS = Number(process.argv[2] || 45000);

(async () => {
  const b = await chromium.connectOverCDP("http://127.0.0.1:9222");
  const page = b.contexts()[0].pages()[0];
  await page.evaluate(() => {
    window.__v = { writes: [] };
    window.__REASONIX_TRANSCRIPT_SCROLL_WRITE__ = (w) => {
      window.__v.writes.push({ owner: w.owner, top: Math.round(w.top ?? -1), t: performance.now() });
    };
  });
  const t0 = Date.now();
  const stats = {
    conversation: { minSh: 1e9, maxSh: 0, stRev: 0, lastSt: null, lastDir: 0 },
    process: { minSh: 1e9, maxSh: 0, stRev: 0, lastSt: null, lastDir: 0 },
  };
  while (Date.now() - t0 < MAX_MS) {
    const s = await page.evaluate(() => {
      const g = (sel) => {
        const el = document.querySelector(sel);
        if (!el) return null;
        return { st: Math.round(el.scrollTop), sh: el.scrollHeight, gap: Math.round((el.scrollHeight - el.clientHeight - el.scrollTop) * 10) / 10 };
      };
      return { c: g(".conversation-pane"), p: g(".process-pane") };
    });
    for (const [k, key] of [["c", "conversation"], ["p", "process"]]) {
      const v = s[k];
      const st = stats[key];
      if (!v) continue;
      st.minSh = Math.min(st.minSh, v.sh);
      st.maxSh = Math.max(st.maxSh, v.sh);
      if (st.lastSt !== null) {
        const d = v.st - st.lastSt;
        if (Math.abs(d) >= 4) {
          const dir = d > 0 ? 1 : -1;
          if (st.lastDir !== 0 && dir !== st.lastDir) st.stRev += 1;
          st.lastDir = dir;
        }
      }
      st.lastSt = v.st;
    }
    await page.waitForTimeout(30);
  }
  const writes = await page.evaluate(() => {
    const w = window.__v.writes;
    delete window.__REASONIX_TRANSCRIPT_SCROLL_WRITE__;
    delete window.__v;
    return w;
  });
  const summarize = (key, st) => {
    const osc = st.maxSh - st.minSh;
    const own = writes.filter((w) => (key === "conversation" ? w.owner === "tail-follow" : true));
    return { minSh: st.minSh, maxSh: st.maxSh, oscillation: osc, scrollTopReversals: st.stRev };
  };
  const totalWrites = writes.length;
  const rateHz = totalWrites / (MAX_MS / 1000);
  console.log(JSON.stringify({
    windowSeconds: MAX_MS / 1000,
    conversation: summarize("conversation", stats.conversation),
    process: summarize("process", stats.process),
    totalWrites,
    writeRateHz: Math.round(rateHz * 10) / 10,
    writes: writes.slice(0, 40).map((w) => ({ owner: w.owner, top: w.top })),
  }, null, 1));
  await b.close();
})().catch((e) => { console.error(e); process.exit(1); });
