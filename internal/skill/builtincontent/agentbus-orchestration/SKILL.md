---
name: agentbus-orchestration
description: "Write a task as a node graph on the agent blackboard: deliverable roots, a node granularity that someone else can re-run, subtrees and boundary nodes, claiming and handing out steps, settling disputes, and what a survivor does after the orchestrator is gone. Use when asked to run a long task across sessions, to split work for other agents, or to hand a task over through the board."
runAs: inline
---

# Orchestrating work on the board

The rules live in `docs/agents/ORCHESTRATION.md` (Chinese; every rule names the kernel code and the
test that pins it). This is the working checklist.

## The one judgement

Kill the orchestrator: if the work still moves, it is a cluster; if it stops, it was a job dispatcher.
So the **board** carries the truth — nodes, dependencies, evidence, verdicts — never this conversation.

## Shape the graph

1. **Deliverable first.** Landing is judged on the **roots** of the dependency graph, so write the
   deliverable as a node and make it wait for its blocks: `require(node=<deliverable>, dep=<block>)`.
   Reversed, the earliest step becomes the "deliverable" and the task lands early.
2. **One node = one deliverable somebody else can re-run.** `decide(done)` demands checkable evidence
   plus a reproducer who is not the producer. Write that command / file / URL on the node when you
   create it, not when you close it.
3. **Create structure with `require` / `split` (they carry `title`), not `assert`.** `assert` makes an
   id-only node (known boundary) — use it to assert on nodes that already exist.
4. **One subtree = one deliverable.** Subtree ownership (budget, affinity) is derived by walking
   dependencies up, so splitting one deliverable across two subtrees splits the accounting.
5. **Cross subtrees only through a boundary node.** The board view is cropped (≤200 lines, ≤8 KiB):
   a dependency on somebody else's internal step can be invisible to you, and can be split away or
   reverted from under you.

## Run it

1. `agent_bus{action=view}` first — it shows only what concerns you.
2. `claim` needs `steps` and `leaseSeconds`. Long work: `heartbeat` to renew, or the board takes the
   claim back (a real machine showed 27 minutes of nothing when nobody renewed).
3. After `split` — and after `require` for work you will do yourself — **claim in the very next op**:
   a fresh child has no lease, and the dispatcher hands startable work to someone else within ~30s.
4. Hand off with `assign` (names one participant; only they may take it) or give it back with `unassign`.
5. Disagreement: `refute` with a reason, then open a hearing — `hearing_open` → `hearing_answer`
   (with evidence; an answer that brings nothing checkable weighs nothing) → `hearing_settle`; a
   refuted step ends up `blocked`. A hearing only wakes the side that owes an answer once its round
   window has passed (`[agentbus] hearing_round_ttl_minutes`), which is why that knob has to be set.
6. Finish with `decide{outcome=done|blocked}` plus the evidence. Declaring the session's task complete
   is gated by the landing check: any deliverable not done, or any unresolved dispute, refuses it.

## When the orchestrator is gone

Read the board and **grow the graph** — not just pick up parked steps:

- find the deliverable roots and what they still wait for;
- if a block nothing waits on is missing, `require` it (with a title) and claim it;
- if the shape is wrong, fix it with `revert` (pulls `done` back and marks dependents `stale`),
  `abandon` (needs evidence; it deliberately does not propagate) or `split`;
- the drill that pins this: `internal/agentbus/orchestration_drill_test.go`.

## Anti-patterns

Truth in the conversation. One mega-node that waits for everything. A reversed dependency. A direct
dependency on another subtree's internal step. Naming conventions carrying meaning. `abandon` used as
"cancel". Only consuming the steps somebody else wrote — that is a job dispatcher, not a cluster.
