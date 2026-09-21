// Maps the loaded transcript onto the whole-session audit payload: one entry per
// turn that has a reasoning chain, carrying the session-absolute turn number the
// transcript badge already shows and the request that turn answered.
import { buildTurnModels, NO_LIVE } from "./transcriptRows";
import type { Item } from "./useController";
import type { SessionAuditTurn } from "../generated/desktopContract.generated";

type AssistantItem = Extract<Item, { kind: "assistant" }>;

const HAS_TEXT = /\S/;

// countAuditableTurns answers "is there anything to audit?" without assembling
// the payload, so the trigger can stay disabled on a reasoning-free session
// without paying for a full copy on every transcript update.
export function countAuditableTurns(items: readonly Item[], turnBase = 0): number {
  let count = 0;
  for (const model of buildTurnModels(items, NO_LIVE, false, false, turnBase)) {
    if (model.turnItems.some((item) => item.kind === "assistant" && HAS_TEXT.test(item.reasoning))) count += 1;
  }
  return count;
}

export function collectSessionAuditTurns(items: readonly Item[], turnBase = 0): SessionAuditTurn[] {
  const turns: SessionAuditTurn[] = [];
  for (const model of buildTurnModels(items, NO_LIVE, false, false, turnBase)) {
    const reasoning = model.turnItems
      .filter((item): item is AssistantItem => item.kind === "assistant")
      .map((item) => item.reasoning.trim())
      .filter(Boolean)
      .join("\n\n");
    if (!reasoning) continue;
    turns.push({
      turn: typeof model.turn === "number" ? model.turn + 1 : 0,
      prompt: (model.user?.text ?? "").trim(),
      reasoning,
    });
  }
  return turns;
}
