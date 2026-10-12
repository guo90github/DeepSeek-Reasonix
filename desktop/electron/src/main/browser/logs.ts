import type { Logger } from "../log.js";
import type { GuestPage } from "./guestView.js";

export const CONSOLE_LIMIT = 200;
export const NETWORK_LIMIT = 200;

export interface ConsoleRecord {
  level: string;
  text: string;
  at: number;
}

export interface NetworkRecord {
  method: string;
  url: string;
  status: number;
  mime: string;
  failed: string;
}

export interface LogsDeps {
  now?(): number;
  log: Logger;
}

// TabLogs keeps what the page logged and fetched, in memory only: a ring
// buffer per tab, never written to disk. Recording starts with the first
// request, because attaching the debugger is visible to the page.
export class TabLogs {
  private console: ConsoleRecord[] = [];
  private network = new Map<string, NetworkRecord>();
  private attached = false;
  private failed = "";

  constructor(private readonly deps: LogsDeps) {}

  attach(page: GuestPage): void {
    if (this.attached) return;
    const dbg = page.debugger;
    this.attached = true;
    try {
      if (!dbg.isAttached()) dbg.attach("1.3");
    } catch (error) {
      this.failed = `the debugger could not attach: ${String(error)}`;
      this.deps.log.warn(`browser logs unavailable: ${this.failed}`);
      return;
    }
    dbg.on("message", (_event, method, params) => this.record(method, params));
    void dbg.sendCommand("Runtime.enable").catch((error: unknown) => this.warn("Runtime.enable", error));
    void dbg.sendCommand("Network.enable").catch((error: unknown) => this.warn("Network.enable", error));
  }

  consoleRecords(): ConsoleRecord[] {
    return [...this.console];
  }

  networkRecords(): NetworkRecord[] {
    return [...this.network.values()];
  }

  clear(): void {
    this.console = [];
    this.network.clear();
  }

  private warn(method: string, error: unknown): void {
    this.failed = `${method} failed: ${String(error)}`;
    this.deps.log.warn(`browser logs: ${this.failed}`);
  }

  private record(method: string, params: unknown): void {
    const at = (this.deps.now ?? Date.now)();
    const body = (params ?? {}) as Record<string, unknown>;
    switch (method) {
      case "Runtime.consoleAPICalled":
        this.push({ level: String(body.type ?? "log"), text: describeArgs(body.args), at });
        break;
      case "Runtime.exceptionThrown":
        this.push({ level: "error", text: describeException(body.exceptionDetails), at });
        break;
      case "Network.requestWillBeSent":
        this.startRequest(body);
        break;
      case "Network.responseReceived":
        this.answerRequest(body);
        break;
      case "Network.loadingFailed":
        this.failRequest(body);
        break;
      default:
        break;
    }
  }

  private push(entry: ConsoleRecord): void {
    this.console.push(entry);
    if (this.console.length > CONSOLE_LIMIT) this.console.splice(0, this.console.length - CONSOLE_LIMIT);
  }

  private startRequest(body: Record<string, unknown>): void {
    const id = String(body.requestId ?? "");
    const request = (body.request ?? {}) as Record<string, unknown>;
    if (id === "") return;
    this.network.set(id, { method: String(request.method ?? "GET"), url: String(request.url ?? ""), status: 0, mime: "", failed: "" });
    if (this.network.size > NETWORK_LIMIT) {
      const oldest = this.network.keys().next();
      if (!oldest.done) this.network.delete(oldest.value);
    }
  }

  private answerRequest(body: Record<string, unknown>): void {
    const entry = this.network.get(String(body.requestId ?? ""));
    if (!entry) return;
    const response = (body.response ?? {}) as Record<string, unknown>;
    entry.status = Number(response.status ?? 0);
    entry.mime = String(response.mimeType ?? "");
  }

  private failRequest(body: Record<string, unknown>): void {
    const entry = this.network.get(String(body.requestId ?? ""));
    if (!entry) return;
    entry.failed = String(body.errorText ?? "failed");
  }
}

export function describeArgs(args: unknown): string {
  if (!Array.isArray(args)) return "";
  const parts = args.map((arg) => {
    const value = arg as Record<string, unknown> | null;
    if (value && typeof value === "object" && "value" in value) return stringify(value.value);
    if (value && typeof value === "object" && "description" in value) return String(value.description);
    return "";
  });
  return parts.filter((part) => part !== "").join(" ");
}

export function describeException(details: unknown): string {
  const body = (details ?? {}) as Record<string, unknown>;
  const thrown = body.exception as Record<string, unknown> | undefined;
  const text = thrown && "description" in thrown ? String(thrown.description) : String(body.text ?? "uncaught exception");
  return text.split("\n")[0] ?? text;
}

function stringify(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return String(value);
  if (typeof value === "object") {
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  return String(value);
}
