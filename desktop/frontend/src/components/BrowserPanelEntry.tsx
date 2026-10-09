import "./BrowserPanel.css";

import { BrowserDockTab, BrowserPanel, BrowserWindowEntry } from "./BrowserPanel";

export type BrowserSurfaceProps =
  | { surface: "tab"; active: boolean; onSelect: () => void }
  | { surface: "panel"; taskId: string | undefined }
  | { surface: "entry" };

// One lazy boundary for the whole dock surface: the tab button, the panel and
// the window entry share this chunk, so the initial bundle carries no browser
// code at all.
export default function BrowserSurface(props: BrowserSurfaceProps) {
  if (props.surface === "tab") return <BrowserDockTab active={props.active} onSelect={props.onSelect} />;
  return props.surface === "entry" ? <BrowserWindowEntry /> : <BrowserPanel taskId={props.taskId} />;
}
