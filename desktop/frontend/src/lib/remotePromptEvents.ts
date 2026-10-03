import { desktopHost } from "./desktopHost";

type UserInsert = { type: "user"; text: string; seq: number; submissionId: string };

type SessionFields = {
  sessionPath?: string;
  sessionRevision?: number;
  sessionDigest?: string;
  sessionGeneration?: number;
};

export type RemotePromptPorts = {
  read(tabId: string): { seq: number; meta?: SessionFields } | undefined;
  dispatch(tabId: string, action: UserInsert): void;
  reload(tabId: string, force: boolean, reason: "startup", options: SessionFields & { recoveryCurrent?: () => boolean }): void;
};

/**
 * 宿主替远端（手机经 /submit）写进某个 tab 转录的那条提问。窗口自己提交的 prompt 会在前端
 * 本地生成 item，所以能渲染；远端那条只存在于转录里，前端 item 流从来没有它 —— 这条信号
 * 就是补这个缺口：先把行插上（立即可见），再从会话重读一次，让权威行替换掉这条乐观行。
 */
export function bindRemotePromptInsertion(ports: RemotePromptPorts): () => void {
  const host = desktopHost();
  if (host.kind === "none") return () => {};
  return host.events.on("prompt:remote-inserted-v1", (payload?: unknown) => {
    const record = payload && typeof payload === "object" ? payload as { tabId?: unknown; text?: unknown } : {};
    const tabId = String(record.tabId ?? "");
    const text = String(record.text ?? "");
    const state = tabId ? ports.read(tabId) : undefined;
    if (!tabId || !text.trim() || !state) return;
    ports.dispatch(tabId, { type: "user", text, seq: state.seq, submissionId: `remote-${tabId}-${state.seq}` });
    window.setTimeout(() => {
      const meta = ports.read(tabId)?.meta;
      if (!meta?.sessionPath) return;
      ports.reload(tabId, true, "startup", { ...meta, recoveryCurrent: () => true });
    }, 500);
  });
}
