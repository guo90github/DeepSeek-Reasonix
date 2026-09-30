import { clampWorkspaceSplitTreeWidth } from "./workspaceSplit";

// The panel's own geometry and the request shapes its children speak. Both are
// private to WorkspacePanel, which is large enough that its scaffolding should
// not compete with its layout for the reader's attention.
export const WORKSPACE_TREE_MIN_WIDTH = 140;
export const WORKSPACE_TREE_DEFAULT_WIDTH = 300;
export const WORKSPACE_PREVIEW_MIN_WIDTH = 140;
export const WORKSPACE_PREVIEW_TARGET_WIDTH = 360;
export const WORKSPACE_DUAL_PANEL_TARGET_WIDTH = WORKSPACE_TREE_DEFAULT_WIDTH + WORKSPACE_PREVIEW_TARGET_WIDTH;
export const WORKSPACE_CONTEXT_MENU_SELECTION_HEIGHT = 48;
export const WORKSPACE_MAX_PREVIEW_TABS = 5;

export type WorkspaceRevealRequest = { id: number; path: string };
export type WorkspaceFileListRequest = { id: number; paths: string[] };
export type WorkspaceChangeListEntry = { key: string; path: string; meta: string; time: string; detail: string };
export type WorkspaceChangeListRequest = { id: number; changes: WorkspaceChangeListEntry[] };

export function clampWorkspaceTreeWidth(width: number, panelWidth?: number): number {
  return clampWorkspaceSplitTreeWidth({
    width,
    panelWidth,
    treeMinWidth: WORKSPACE_TREE_MIN_WIDTH,
    previewMinWidth: WORKSPACE_PREVIEW_MIN_WIDTH,
  });
}
