// 崩溃恢复自己的开关，紧挨无人值守总开关渲染在 tab 条里。两者相关但**互相独立**：总开关持有
// 登录项，这一枚持有那条把死掉的宿主拉回来的 OS 条目（docs/UNATTENDED.md §10）——手机要够到一台
// 已经不在运行的桌面端，靠的正是它。
//
// 可见标签用名词（「崩溃恢复」）而不是状态词：两枚同款开关并排时，"已开启/已停用"这种标签会让人
// 分不清说的是哪一个；状态交给开关位置 + tooltip + aria-checked。
import { useCallback, useEffect, useState } from "react";
import { Tooltip } from "../../../components/Tooltip";
import { heartbeatWatchdogSet, heartbeatWatchdogStatus } from "./heartbeat.bridge";
import { useHeartbeatT } from "./heartbeat.i18n";
import { watchdogTogglePresentation, type UnattendedWatchdogState } from "./heartbeat.presentation";

export function WatchdogToggle() {
  const t = useHeartbeatT();
  const [state, setState] = useState<UnattendedWatchdogState | null>(null);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    let live = true;
    void heartbeatWatchdogStatus().then((value) => {
      if (live) setState(value);
    });
    return () => {
      live = false;
    };
  }, []);

  const flip = useCallback(() => {
    if (state === null || pending) return;
    setPending(true);
    void heartbeatWatchdogSet(state.policy === false)
      .then((value) => setState(value), () => undefined)
      .finally(() => setPending(false));
  }, [state, pending]);

  // 这个平台没有 OS 集成就没有这枚开关（不是"关着"）：不假装能开一个不存在的东西。
  if (state === null || state.supported === false) return null;
  const presentation = watchdogTogglePresentation(state);
  const name = t("heartbeat.watchdog");
  const label = t(presentation.stateKey);
  const hint = t(presentation.hintKey, presentation.hintParams);
  return (
    <Tooltip label={`${name} · ${label} — ${hint}`}>
      <button
        type="button"
        className="tabbar__unattended"
        role="switch"
        aria-checked={presentation.on}
        aria-label={`${name}：${label}`}
        data-crash-recovery={presentation.on ? "on" : "off"}
        disabled={pending}
        onClick={flip}
      >
        <span className="tabbar__unattended-track" aria-hidden="true">
          <span className="tabbar__unattended-knob" />
        </span>
        <span className="tabbar__unattended-label">{name}</span>
      </button>
    </Tooltip>
  );
}
