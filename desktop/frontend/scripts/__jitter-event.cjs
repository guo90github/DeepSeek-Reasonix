// Jitter event catcher: 30ms sampling of both panes with a rolling ~1.2s
// trace. On a st direction-reversal burst (2 reversals within 700ms — the
// visible up-down jitter) in either pane, dump the trace + writes around the
// event. Runs up to 10 min. Self-contained; deletes sink on exit.
const { createRequire } = require("module");
const req = createRequire("C:/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend/package.json");
const { chromium } = req("playwright");

(async () => {
  const b = await chromium.connectOverCDP("http://127.0.0.1:9222");
  const page = b.contexts()[0].pages()[0];
  const t0 = Date.now();
  const state = {
    conversation: { trace: [], lastSt: null, lastDir: 0, revAt: 0, revs: 0, lastW: 0 },
    process: { trace: [], lastSt: null, lastDir: 0, revAt: 0, revs: 0, lastW: 0 },
  };
  let writeLog = [];
  await page.evaluate(() => {
    window.__j = { w: [] };
    window.__REASONIX_TRANSCRIPT_SCROLL_WRITE__ = (x) => {
      window.__j.w.push({ owner: x.owner, top: Math.round(x.top ?? -1), t: performance.now() });
    };
  });

  while (Date.now() - t0 < 600000) {
    const s = await page.evaluate(() => {
      const g = (sel) => {
        const el = document.querySelector(sel);
        if (!el) return null;
        return { st: Math.round(el.scrollTop), sh: el.scrollHeight, gap: Math.round((el.scrollHeight - el.clientHeight - el.scrollTop) * 10) / 10 };
      };
      return { c: g(".conversation-pane"), p: g(".process-pane"), w: window.__j.w.length };
    });
    const now = Date.now();
    for (const [k, name] of [["c", "conversation"], ["p", "process"]]) {
      const v = s[k];
      const st = state[name];
      if (!v) continue;
      st.trace.push({ t: now - t0, st: v.st, sh: v.sh, gap: v.gap });
      if (st.trace.length > 40) st.trace.shift(); // ~1.2s
      if (st.lastSt !== null) {
        const d = v.st - st.lastSt;
        if (Math.abs(d) >= 4) {
          const dir = d > 0 ? 1 : -1;
          if (st.lastDir !== 0 && dir !== st.lastDir) {
            if (now - st.revAt < 700) {
              st.revs += 1;
              if (st.revs >= 2) {
                // caught a jitter event
                const writes = await page.evaluate(() => {
                  const w = window.__j.w;
                  delete window.__REASONIX_TRANSCRIPT_SCROLL_WRITE__;
                  delete window.__j;
                  return w;
                });
                writeLog = writes;
                console.log("JITTER EVENT", name, "at", ((now - t0) / 1000).toFixed(1) + "s");
                console.log("trace:", JSON.stringify(st.trace.map((x) => `t=${(x.t / 1000).toFixed(2)} st=${x.st} sh=${x.sh} gap=${x.gap}`)));
                const allW = writeLog.slice(-40).map((w) => `+${((w.t - (writeLog[0]?.t ?? w.t)) / 1000).toFixed(2)}s ${w.owner} top=${w.top}`);
                console.log("writes:", allW.join(" "));
                await b.close();
                return;
              }
            } else {
              st.revs = 1;
            }
            st.revAt = now;
          } else {
            st.revs = 0;
          }
          st.lastDir = dir;
        }
      }
      st.lastSt = v.st;
    }
    await page.waitForTimeout(30);
  }
  console.log("no jitter event in 10 min");
  await b.close();
})().catch((e) => { console.error(e); process.exit(1); });
