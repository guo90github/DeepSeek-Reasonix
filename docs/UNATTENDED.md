# Unattended long tasks: keep going, keep alive, self-heal

<a href="./UNATTENDED.zh-CN.md">中文</a>

What "turn the master switch on and let the host push a long task by itself"
means: the semantics, the keep-alive chain, and the known limits. Everything
lives in `desktop/` (host, Electron shell, frontend); the kernel
(`internal/control`) is untouched.

## 1. The only gate: the master switch

- Where: `unattended` at the top level of `<Reasonix home>/heartbeat-tasks.json`.
  The desktop exposes it as a small pill left of the tab strip's new-session
  button, whose tooltip states when it applies.
- **It applies on restart.** A running process snapshots it once in
  `HeartbeatEngine.Start()` (`e.unattended`); reading or writing the file never
  changes what the current process does.
- Switch off: nothing changes — tasks with a `goal` behave like plain scheduled
  prompts, and no unattended code runs.

## 2. Task model: the `goal` contract

| Field | Meaning |
|---|---|
| `goal` | Non-empty = this task is an unattended long task, and the text is its **goal contract**; empty = today's plain scheduled prompt |
| `topicId` | The task's one session binding (the driver touches nothing else) |
| `interval` | The only rate limit: how often it takes a look |

Decision table (`heartbeatGoalDecide`, a pure function):

| Session state | Action |
|---|---|
| Task has no `goal` | Submit the prompt (plain task) |
| No Goal anchored | **Anchor** the contract, then submit |
| Goal ≠ contract | **Re-anchor to the contract** (logged), then submit |
| Goal running | Yield this tick (the Goal's own continuation drives), re-check next interval |
| Goal complete | Stop submitting (the work is done) |
| Goal blocked/stopped | **Resume without a ceiling**, once per interval |
| Switch off | Fully inert |

### "A human took over" does not exist

The product decision: **the master switch is the only gate.** Even if someone
intervenes in that session while the switch is on — including setting a different
Goal — everything stays unattended, and the driver re-anchors the session's Goal
to the task contract instead of yielding. There is deliberately no takeover
heuristic based on human activity, turn origin, or last-activity time; do not
add one back "for politeness".

Ownership comes from the binding: the driver only ever acts on the session its
`topicId` names.

## 3. Launch marker and unclean-exit detection

`<Reasonix home>/desktop-host-state.json` (`host_state_marker.go`):

```json
{ "schemaVersion": 1, "pid": 1234, "runId": "…", "version": "v0.0.0-dev.N",
  "phase": "running", "unattended": true, "startedAt": "…", "updatedAt": "…",
  "uncleanStreak": 1 }
```

- Written on every launch, **independent of telemetry** (`diagnostics/lifecycle/*`
  is not written on dev builds or with telemetry off, so it cannot be the
  criterion).
- A clean exit (`completeDesktopShutdown`) removes it.
- Therefore: **a leftover marker whose PID is dead means the last run died**; its
  `unattended` flag is the desired state the restart policy and any external
  watchdog read.
- `uncleanStreak` counts consecutive unclean exits inside 10 minutes; at **3 the
  launch degrades** — the driver stays off (`Start()` ANDs it with the switch) so
  a crashing task cannot turn into a crash storm.

## 4. Keep-alive chain

| Layer | Mechanism | Behaviour |
|---|---|---|
| Service child | Electron `ServiceSupervisor` | Attended: 3 restarts / 5 min; **unattended: unlimited, exponential backoff** (1s→…→60s cap) |
| Whole shell | `app.relaunch()` | When retries are exhausted and unattended, relaunch and exit; a 3 / 15 min budget keeps a broken build from looping |
| Boot | login item (**opt-in**) | Registered only when `<home>/desktop-autostart.json` says `enabled: true`; disabling unregisters it, and nothing ever touches a machine that did not ask |
| Session state | marker + goal sidecar | Sessions and Goals recover on restart; the driver continues next interval |

A deliberate quit never relaunches anything: a clean exit removes the marker, so
neither the restart policy nor a watchdog sees a desired state.

## 5. Self-healing the stops

While the switch is on and the task has a `goal`, the gates that only a human
could answer are cleared:

- **plan mode**: forced off (its approval gate would wait forever).
- **paused inbox**: resumed (`SetInboxPaused(false)`) — inbox recovery pauses the
  queue until someone looks.

## 6. Spent window → handoff session

- Criterion: `ContextSnapshot()` usage ≥ **96%** (`heartbeatWindowSpent`).
- Action: open a **fresh session**, switch `topicId` to it, and give it a one-shot
  preface carrying the goal contract and **the old transcript's path**, so the
  model reads what it needs instead of the driver inventing a summary.
- The new session anchors the same `goal` contract and continues; the old session
  stays on disk, readable.

## 7. Code map

| File | Responsibility |
|---|---|
| `desktop/heartbeat.go` | Task model (`goal`/`unattended`), engine wiring, one-shot handoff preface |
| `desktop/heartbeat_converge.go` | The driver: decision table, anchor/resume/yield, gate clearing |
| `desktop/heartbeat_handoff.go` | Spent-window decision and the handoff session |
| `desktop/host_state_marker.go` | Launch marker and crash-streak judgement |
| `desktop/heartbeat_store.go` | Config read/write (the switch is human-owned; a full-table save must not drop it) |
| `desktop/electron/src/main/{hostState,restartPolicy,autostart}.ts` | Marker reads, restart backoff, opt-in autostart |
| `desktop/frontend/src/custom/features/heartbeat/UnattendedToggle.tsx` | The tab-strip switch |
| `docs/UNATTENDED*.md` | This document |

## 8. Verification

```bash
cd desktop && go test -run 'TestHeartbeat|TestHostState' -count=1 .   # driver/marker/handoff
cd desktop && go test -count=1 .                                     # full suite (see noise below)
cd desktop/electron && npm test && npm run typecheck                 # shell-side policy
go run ./tools/repolint                                              # comment/size gates
```

Pre-existing noise, unrelated to this work:
`TestDesktopBuildScriptCompilesAndPackagesWindowsUpdateHelper` asserts the
contents of `scripts/desktop-build.sh`; the electron suite has 5 Windows
path-separator failures.

## 9. Limits and open items

- The old session's `scopeID`, DeliveryCheckpoint and todos are **not** migrated:
  the new session re-anchors by the same `goal` text, so the delivery-evidence
  chain loses one link.
- No macOS `LaunchAgent KeepAlive`: if Electron main itself dies and the relaunch
  budget is spent, the shell parks on the failure page (deliberate — never loop).
- The panel does not yet display the switch's current value (the config file is
  authoritative).
- The full chain — kill the process abnormally → it relaunches → the long task
  continues — has not been accepted on a real machine yet.
