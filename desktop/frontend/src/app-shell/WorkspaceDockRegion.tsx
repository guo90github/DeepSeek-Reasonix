import { lazy, Suspense, type ComponentProps, type KeyboardEvent, type PointerEvent, type ReactNode } from "react";
import { Activity, FileText, Server } from "lucide-react";
import { desktopHost } from "../lib/desktopHost";
import type { Translator } from "../lib/i18n";
import type { RightDockMode } from "../store/layout";
import { DockToggleButton } from "./DockToggleButton";

const ContextPanel = lazy(() => import("../components/ContextPanel").then((module) => ({ default: module.ContextPanel })));
const RemotePanel = lazy(() => import("../components/RemotePanel").then((module) => ({ default: module.RemotePanel })));
const BrowserSurface = lazy(() => import("../components/BrowserPanelEntry"));
const WorkspacePanel = lazy(async () => {
  const [module] = await Promise.all([
    import("../components/WorkspacePanel"),
    import("../components/WorkspacePanelStability.css"),
  ]);
  return { default: module.WorkspacePanel };
});

export type WorkspaceDockRegionProps = {
  visible: boolean;
  overlay: boolean;
  mode: RightDockMode;
  creation: boolean;
  remoteAvailable: boolean;
  showContext: boolean;
  t: Translator;
  onMode: (mode: RightDockMode) => void;
  onRemote: () => void;
  onClose?: () => void;
  remote: ComponentProps<typeof RemotePanel>;
  context: ComponentProps<typeof ContextPanel>;
  workspace: ComponentProps<typeof WorkspacePanel>;
  workspaceKey: string;
  resizer?: {
    min: number;
    max: number;
    value: number;
    onPointerDown: (event: PointerEvent<HTMLButtonElement>) => void;
    onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void;
    onReset: () => void;
  };
};

/**
 * Workbench dock. Non-creation keeps the overview single tab: context/changed
 * (and files requests) share one merged body — ContextPanel on top, the
 * workspace panel with its own files/changed view tabs filling the rest — so
 * 文件/改动 stay inside the 概览 tab instead of being top-level tabs. Creation
 * shows only the files tab and the plain workspace panel. A shell that exposes
 * an embedded browser adds its tab beside them; that tab carries the browser
 * window's status and the action that opens it, because the pages themselves
 * live in their own window.
 */
export function WorkspaceDockRegion(props: WorkspaceDockRegionProps) {
  const { visible, overlay, mode, creation, remoteAvailable, showContext, t, onMode, onRemote, onClose } = props;
  const browser = !creation && desktopHost().browser !== undefined;
  const merged = !creation && mode !== "remote" && mode !== "browser";
  return (
    <>
      {props.resizer && (
        <button
          className="workspace-panel-resizer" type="button" role="separator" aria-orientation="vertical"
          aria-label={t("rightDock.resize")} aria-valuemin={props.resizer.min}
          aria-valuemax={props.resizer.max} aria-valuenow={props.resizer.value}
          onPointerDown={props.resizer.onPointerDown} onKeyDown={props.resizer.onKeyDown}
          onDoubleClick={props.resizer.onReset}
        />
      )}
      {visible && (
        <aside className={["workbench-dock", `workbench-dock--${mode}`, overlay ? "workbench-dock--overlay" : ""].join(" ")} aria-label={t("rightDock.workbench")}>
          <div className="workbench-dock__tools">
            <div className="workbench-dock__tabs" role="tablist" aria-label={t("rightDock.views")}>
              {showContext && !creation && <DockTab active={mode !== "remote" && mode !== "browser"} onClick={() => onMode("context")} icon={<Activity size={13} />} label={t("rightDock.overview")} />}
              {creation && <DockTab active={mode === "files"} onClick={() => onMode("files")} icon={<FileText size={13} />} label={t("workspace.filesTab")} />}
              {browser && (
                <Suspense fallback={null}>
                  <BrowserSurface surface="tab" active={mode === "browser"} onSelect={() => onMode("browser")} />
                </Suspense>
              )}
              {remoteAvailable && <DockTab active={mode === "remote"} onClick={onRemote} icon={<Server size={13} />} label={t("rightDock.remote")} />}
            </div>
            {onClose && (
              <div className="workbench-dock__collapse">
                <DockToggleButton renderable={true} t={t} onToggle={onClose} />
              </div>
            )}
          </div>
          <div className={["workbench-dock__body", merged ? "workbench-dock__body--merged" : ""].filter(Boolean).join(" ")}>
            {mode === "remote" ? (
              <Suspense fallback={null}><RemotePanel {...props.remote} /></Suspense>
            ) : mode === "browser" ? (
              <Suspense fallback={null}><BrowserSurface surface="entry" /></Suspense>
            ) : merged ? (
              <>
                <Suspense fallback={null}><ContextPanel {...props.context} /></Suspense>
                <Suspense fallback={null}><WorkspacePanel key={props.workspaceKey} {...props.workspace} /></Suspense>
              </>
            ) : (
              <Suspense fallback={null}><WorkspacePanel key={props.workspaceKey} {...props.workspace} /></Suspense>
            )}
          </div>
        </aside>
      )}
    </>
  );
}

function DockTab({ active, onClick, icon, label }: { active: boolean; onClick: () => void; icon: ReactNode; label: string }) {
  return (
    <button type="button" role="tab" aria-selected={active} className={`workbench-dock__tab${active ? " workbench-dock__tab--active" : ""}`} onClick={onClick}>
      {icon}<span className="workbench-dock__tab-label">{label}</span>
    </button>
  );
}
