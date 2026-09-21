// Mock binding for the whole-session audit. The dev/mock flow (?mock=demo|bench)
// has no audit model behind it, so this synthesises the same step stream the host
// emits (per-segment requests, streamed verdict text, the cross-turn review, then
// the verdict) and lets the modal be exercised — and inspected — without a model.
import type { SessionAuditTotals, SessionAuditTurn, SessionAuditTurnResult } from "../generated/desktopContract.generated";
import type { SessionAuditEvent } from "./sessionAuditStream";
import { __emitMockSessionAuditDone, __emitMockSessionAuditEvent } from "./sessionAuditStream";

const MOCK_SEGMENT_PROMPT = "（mock）你是思考链质量审阅者。本次输入是同一会话中连续多轮的思考过程…";
const MOCK_REVIEW_PROMPT = "（mock）你是会话级思考质量会审者。输入是各轮思考链的结构化评审结果…";
const MOCK_SEGMENT_MAX_TURNS = 8;
const MOCK_MAX_SEGMENTS = 8;
const MOCK_TAB_ID = "";

const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function mockScores(turn: number): { score: number; issues: number; counts: number[]; conclusion: string; explanation: string } {
  const score = [0.92, 0.78, 0.55, 0.86, 0.41, 0.9][turn % 6];
  const invalid = turn % 3 === 2 ? 1 : 0;
  const drift = turn % 4 === 3 ? 1 : 0;
  const redundancy = turn % 5 === 4 ? 1 : 0;
  return {
    score,
    issues: invalid + drift + redundancy,
    counts: [invalid, drift, redundancy],
    conclusion: `（mock）第 ${turn} 轮结论：先确认改动边界，再落地实现。`,
    explanation: "（mock）覆盖了该轮要求，存在可定位的推理步骤问题。",
  };
}

function mockTurnRows(turns: SessionAuditTurn[]): SessionAuditTurnResult[] {
  return turns.map((turn) => {
    const info = mockScores(turn.turn);
    return {
      turn: turn.turn,
      score: info.score,
      contradiction: 0,
      factualError: 0,
      invalidInference: info.counts[0],
      redundancy: info.counts[2],
      instructionDrift: info.counts[1],
      omission: 0,
      issues: info.issues,
      conclusion: info.conclusion,
      priorConflict: turn.turn % 5 === 4 ? `（mock）与轮 ${Math.max(1, turn.turn - 1)} 已确认的边界相反` : "",
      explanation: info.explanation,
      findings: info.counts[0] > 0
        ? [{ type: "invalid_inference", quote: "（mock）先按旧接口调用，随后才发现签名已变" }]
        : [],
      truncated: turn.turn % 6 === 0,
    };
  });
}

export function makeMockSessionAuditBinding() {
  return {
    async AuditSession(turns: SessionAuditTurn[]): Promise<SessionAuditTotals> {
      const audited = turns.filter((turn) => turn.reasoning.trim() !== "");
      const groups: SessionAuditTurn[][] = [];
      const groupCount = Math.max(1, Math.min(MOCK_MAX_SEGMENTS, Math.ceil(audited.length / MOCK_SEGMENT_MAX_TURNS)));
      const base = Math.ceil(audited.length / groupCount) || 1;
      for (let index = 0; index < audited.length; index += base) groups.push(audited.slice(index, index + base));
      const total = groups.length + 1;

      for (let index = 0; index < groups.length; index += 1) {
        const group = groups[index];
        const from = group[0]?.turn ?? 0;
        const to = group[group.length - 1]?.turn ?? 0;
        emit({ stage: "segment", index: index + 1, total, kind: "request", systemPrompt: MOCK_SEGMENT_PROMPT, input: `[轮 ${from}] 用户要求：…\n思考过程：\n${group[0]?.reasoning.slice(0, 400) ?? ""}`, turnFrom: from, turnTo: to });
        for (const chunk of ["{\"turns\":[", `{"turn":${from},"score":0.9}`, ", …]}"] ) {
          await delay(80);
          emit({ stage: "segment", index: index + 1, total, kind: "text", text: chunk });
        }
        await delay(120);
        emit({ stage: "segment", index: index + 1, total, kind: "step_done", turnFrom: from, turnTo: to });
      }

      emit({ stage: "review", index: total, total, kind: "request", systemPrompt: MOCK_REVIEW_PROMPT, input: "turn 1 | score 0.92 | - | conclusion … | prior_conflict \"\" | quotes []" });
      for (const chunk of ["{\"score\":0.71,", "\"trend\":\"degrading\",", "\"issues\":[…]}"]) {
        await delay(80);
        emit({ stage: "review", index: total, total, kind: "text", text: chunk });
      }
      await delay(120);
      emit({ stage: "review", index: total, total, kind: "step_done" });

      const rows = mockTurnRows(audited);
      const totals: SessionAuditTotals = {
        audited: true,
        elapsedMs: 4200,
        score: 0.71,
        trend: "degrading",
        explanation: "（mock）后段两轮推翻了早前确认的接口边界，且轮 3 的错误结论被当作前提继续使用；整体结论仍可用但可靠性下降。",
        issues: [
          { type: "cross_turn_contradiction", turns: [3, 5], note: "（mock）轮 5 推翻了轮 3 已确认的调用约定。", quote: "（mock）先按旧接口调用，随后才发现签名已变" },
          { type: "unmet_commitment", turns: [2, 4], note: "（mock）轮 2 承诺补跑回归，后续轮次未再提及。", quote: "" },
        ],
        turns: rows,
        turnCount: rows.length,
        segmentCount: groups.length,
        evalTokens: 8123,
        evalCost: 0.0421,
      };
      await delay(160);
      __emitMockSessionAuditDone(MOCK_TAB_ID, totals);
      return totals;
    },
  };
}

function emit(event: SessionAuditEvent): void {
  __emitMockSessionAuditEvent(MOCK_TAB_ID, event);
}
