// Heartbeat panel bridge — typed wrappers around app heartbeat bindings.
// Custom components should import from here instead of calling app.* directly
// so that heartbeat-specific calls are scoped to this feature.

import { app } from "../../../lib/bridge";
import type { HeartbeatTask } from "./heartbeat.types";
import type { UnattendedWatchdogState } from "./heartbeat.presentation";

interface HeartbeatConfigView {
  revision: number;
  etag: string;
  unattended?: boolean;
  /** Whether the host has any agentbus ceiling at all, straight from the host. */
  agentBusBudget?: boolean;
  tasks: HeartbeatTask[];
	unattendedDriving?: boolean;
	unattendedHold?: string;
}

/** What the switch has to say: the switch itself, and whether anything bounds a run. */
export interface HeartbeatUnattendedState {
  on: boolean;
  budgeted: boolean;
}

let loadedConfigToken: Pick<HeartbeatConfigView, "revision" | "etag"> | null = null;
let loadedTasks: HeartbeatTask[] = [];
let loadedUnattended = false;
let loadedUnattendedDriving = false;
let loadedUnattendedHold = "";
let loadedAgentBusBudget = false;
let configQueue: Promise<void> = Promise.resolve();

function enqueueConfigOperation<T>(operation: () => Promise<T>): Promise<T> {
  const result = configQueue.then(operation, operation);
  configQueue = result.then(() => undefined, () => undefined);
  return result;
}

async function reloadConfig(): Promise<HeartbeatTask[]> {
  const raw = await app.HeartbeatReloadConfig();
  const view = (raw ?? { revision: 0, etag: "", tasks: [] }) as HeartbeatConfigView;
  loadedConfigToken = { revision: view.revision || 0, etag: view.etag || "" };
  loadedTasks = Array.isArray(view.tasks) ? view.tasks : [];
  loadedUnattended = Boolean(view.unattended);
	loadedUnattendedDriving = view.unattendedDriving === true;
	loadedUnattendedHold = view.unattendedHold || "";
  loadedAgentBusBudget = Boolean(view.agentBusBudget);
  return loadedTasks;
}

async function saveConfig(tasks: HeartbeatTask[]): Promise<HeartbeatTask[]> {
  const view = await app.HeartbeatSaveConfig({
    revision: loadedConfigToken?.revision || 0,
    etag: loadedConfigToken?.etag || "",
    tasks,
  });
  const saved = (view ?? { revision: 0, etag: "" }) as HeartbeatConfigView;
  loadedConfigToken = { revision: saved.revision || 0, etag: saved.etag || "" };
  loadedTasks = Array.isArray(saved.tasks) ? saved.tasks : tasks;
  loadedUnattended = Boolean(saved.unattended);
  loadedAgentBusBudget = Boolean(saved.agentBusBudget);
  return loadedTasks;
}

export function heartbeatListTasks(): Promise<HeartbeatTask[]> {
  return enqueueConfigOperation(reloadConfig);
}

/** Reads the unattended master switch and the host's brake. The switch persists in
 *  the config file, while the running app keeps the value it captured at startup. */
export function heartbeatUnattended(): Promise<HeartbeatUnattendedState> {
  return enqueueConfigOperation(async () => {
    await reloadConfig();
    return { on: loadedUnattended, budgeted: loadedAgentBusBudget, driving: loadedUnattendedDriving, hold: loadedUnattendedHold };
  });
}

/** Writes the master switch, leaving the task list untouched. */
export function heartbeatSetUnattended(on: boolean): Promise<HeartbeatUnattendedState> {
  return enqueueConfigOperation(async () => {
    if (!loadedConfigToken) await reloadConfig();
    const raw = await app.HeartbeatSaveConfig({
      revision: loadedConfigToken?.revision || 0,
      etag: loadedConfigToken?.etag || "",
      tasks: loadedTasks.map((task) => ({ ...task })),
      unattended: on,
    });
    const view = (raw ?? { revision: 0, etag: "" }) as HeartbeatConfigView;
    loadedConfigToken = { revision: view.revision || 0, etag: view.etag || "" };
    loadedTasks = Array.isArray(view.tasks) ? view.tasks : loadedTasks;
    loadedUnattended = Boolean(view.unattended);
	loadedUnattendedDriving = view.unattendedDriving === true;
	loadedUnattendedHold = view.unattendedHold || "";
    loadedAgentBusBudget = Boolean(view.agentBusBudget);
    return { on: loadedUnattended, budgeted: loadedAgentBusBudget, driving: loadedUnattendedDriving, hold: loadedUnattendedHold };
  });
}

export function heartbeatMutateTasks(mutate: (tasks: HeartbeatTask[]) => HeartbeatTask[]): Promise<HeartbeatTask[]> {
  return enqueueConfigOperation(async () => {
    if (!loadedConfigToken) await reloadConfig();
    const current = loadedTasks.map((task) => ({ ...task }));
    try {
      return await saveConfig(mutate(current));
    } catch (error) {
      try {
        await reloadConfig();
      } catch {
        // Preserve the original mutation error; the next operation will retry.
      }
      throw error;
    }
  });
}

export function heartbeatTriggerNow(id: string): Promise<void> {
  return app.HeartbeatTriggerNow(id);
}

export function heartbeatGenerateID(): Promise<string> {
  return app.HeartbeatGenerateID();
}

/** The OS watchdog entry is what brings a dead host back: report it next to the switch. */
export function heartbeatWatchdogStatus(): Promise<UnattendedWatchdogState | null> {
	return app.WatchdogStatus().then(
		(view) => ({
			supported: view.supported,
			registered: view.registered === true,
			lastError: view.lastError || "",
			note: view.note || "",
			lastRunAt: view.lastRunAt || "",
		}),
		() => null,
	);
}
