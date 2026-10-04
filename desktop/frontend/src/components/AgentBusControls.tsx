// The human's side of the board: the same verbs the model's agent_bus tool takes, so a
// person and an agent cannot disagree about what claiming or delivering means. Every
// action goes to the board through the host and comes back with the board's own answer
// — what it recorded, or why it refused and what to change — because a panel that
// decided for itself would be a second, silently different board (AGENT_BUS §13.8).

import { useState } from "react";
import { useT } from "../lib/i18n";
import type { AgentBusApplyArgs, AgentBusChildArg } from "../generated/desktopContract.generated";

type Action = "assert" | "require" | "split" | "claim" | "decide" | "refute" | "release";
type Field = "node" | "reason" | "ref" | "depID" | "depTitle" | "children" | "steps" | "outcome" | "reproducedBy";

const ACTIONS: Action[] = ["assert", "require", "split", "claim", "decide", "refute", "release"];

// Which fields an action reads. The verbs are the kernel's; the form only decides which
// of their arguments a person is asked for.
const FIELDS: Record<Action, Field[]> = {
  assert: ["node", "reason", "ref"],
  require: ["node", "depID", "depTitle"],
  split: ["node", "children"],
  claim: ["node", "steps"],
  decide: ["node", "outcome", "ref", "reproducedBy"],
  refute: ["node", "reason"],
  release: ["node"],
};

const OUTCOMES = ["done", "blocked", "abandoned"] as const;

// Children arrive as one line because that is how a person types a small tree:
// "sign-key:install it, migrate:run the migration".
export function parseChildren(raw: string): AgentBusChildArg[] {
  return raw
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean)
    .map((entry) => {
      const at = entry.indexOf(":");
      if (at < 0) return { id: entry, title: "" };
      return { id: entry.slice(0, at).trim(), title: entry.slice(at + 1).trim() };
    })
    .filter((child) => child.id !== "");
}

export function agentBusArgsFor(action: Action, fields: Record<Field, string>): AgentBusApplyArgs {
  const base: AgentBusApplyArgs = {
    action,
    node: fields.node.trim(),
    title: "",
    reason: "",
    outcome: "",
    ref: "",
    reproducedBy: "",
    steps: 0,
    children: [],
    dep: null,
  };
  switch (action) {
    case "assert":
      return { ...base, reason: fields.reason.trim(), ref: fields.ref.trim() };
    case "require":
      return { ...base, dep: { id: fields.depID.trim(), title: fields.depTitle.trim() } };
    case "split":
      return { ...base, children: parseChildren(fields.children) };
    case "claim":
      return { ...base, steps: Number.parseInt(fields.steps, 10) || 0 };
    case "decide":
      return { ...base, outcome: fields.outcome, ref: fields.ref.trim(), reproducedBy: fields.reproducedBy.trim() };
    case "refute":
      return { ...base, reason: fields.reason.trim() };
    default:
      return base;
  }
}

const EMPTY_FIELDS: Record<Field, string> = {
  node: "",
  reason: "",
  ref: "",
  depID: "",
  depTitle: "",
  children: "",
  steps: "",
  outcome: "done",
  reproducedBy: "",
};

export function AgentBusControls({ apply, onApplied }: {
  /** Injected so the form can be tested without the host. */
  apply: (args: AgentBusApplyArgs) => Promise<string>;
  /** Called after a write so the section refetches what the board now says. */
  onApplied?: () => void;
}) {
  const t = useT();
  const [action, setAction] = useState<Action>("assert");
  const [fields, setFields] = useState<Record<Field, string>>(EMPTY_FIELDS);
  const [answer, setAnswer] = useState("");
  const [busy, setBusy] = useState(false);

  const set = (field: Field) => (value: string) => setFields((current) => ({ ...current, [field]: value }));

  async function submit() {
    setBusy(true);
    setAnswer("");
    try {
      const answer = await apply(agentBusArgsFor(action, fields));
      setAnswer(answer);
      setFields(EMPTY_FIELDS);
      onApplied?.();
    } catch (err) {
      setAnswer(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className="agentbus-controls"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="agentbus-controls__head">
        <strong>{t("agentbus.controls.title")}</strong>
        <select aria-label={t("agentbus.controls.action")} value={action} onChange={(event) => setAction(event.target.value as Action)}>
          {ACTIONS.map((name) => (
            <option key={name} value={name}>
              {t(`agentbus.controls.action.${name}` as "agentbus.controls.action.assert")}
            </option>
          ))}
        </select>
      </div>
      {FIELDS[action].map((field) => (
        <label key={field} className="agentbus-controls__field">
          <span>{t(`agentbus.controls.field.${field}` as "agentbus.controls.field.node")}</span>
          {field === "outcome" ? (
            <select aria-label={t("agentbus.controls.field.outcome")} value={fields.outcome} onChange={(event) => set("outcome")(event.target.value)}>
              {OUTCOMES.map((outcome) => (
                <option key={outcome} value={outcome}>
                  {t(`agentbus.controls.outcome.${outcome}` as "agentbus.controls.outcome.done")}
                </option>
              ))}
            </select>
          ) : (
            <input
              aria-label={t(`agentbus.controls.field.${field}` as "agentbus.controls.field.node")}
              inputMode={field === "steps" ? "numeric" : undefined}
              placeholder={t(`agentbus.controls.placeholder.${field}` as "agentbus.controls.placeholder.node")}
              value={fields[field]}
              onChange={(event) => set(field)(event.target.value)}
            />
          )}
        </label>
      ))}
      <button type="submit" disabled={busy}>
        {t("agentbus.controls.submit")}
      </button>
      {answer ? <p className="agentbus-controls__answer" role="status">{answer}</p> : null}
    </form>
  );
}
