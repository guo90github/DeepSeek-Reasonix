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
- Switch off: nothing changes — tasks behave like plain scheduled prompts, and no
  unattended code runs.
- With the switch on, unattended care does **not** depend on a `goal`: the contract
  only says whether there is a Goal to steer. Gate clearing, the spent-window handoff
  and the version switch apply to **every** task.
- The switch also owns the **OS watchdog entry** (§10): turning it on registers the
  watchdog, turning it off unregisters it and removes the desktop file. The entry is
  written at once but, like the switch, protects the **next** launch — the marker
  carries the launch-time value, so that is the run a crash would restore. A refused
  registration is logged, never fails the switch write.

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
| `desktop/portable_upgrade.go` | Version switch: ready gate, pointer swap, restart into the new version |
| `desktop/electron/src/main/upgradeRollback.ts` | Puts a switch that never came up back on the previous version |
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

## 9. Unattended self-upgrade (keep the long task running on the new build)

The task's artifacts land under `versions/<tag>/` with **increasing version
numbers**, and a version's **only launch entry is its own inner
`versions/<tag>/reasonix-desktop.exe`**. The switching rules:

1. **The primary signal is `current.json`, which the packaging SOP maintains**: the
   build flow is governed by this machine's SOP (`REASONIX.local.md`), and it points
   the pointer at the new tag; whenever the pointer names another **complete**
   version, the switch happens at a turn boundary.
2. **Fallback signal**: when the pointer has not moved, take the **newest ready**
   version under `versions/` — ready means `versions/<tag>/.reasonix-upgrade-ready.json`
   with `{"ready":true}`, written by the task after `verify-windows-portable.sh`
   exits 0. A tree without the marker is never switched to: a half-extracted tree
   must not become the active version.
3. **Action**: write `current.json` (`installlayout.WriteCurrent` validates
   `versions/<version>` and rejects traversal and symlinks) → record the rollback
   target (`desktop-upgrade.json`, phase `pending`) → ask the shell to restart into
   the **new version's inner desktop binary**.
4. **Turn boundaries only**: the switch happens when the driver starts a tick and
   before it submits, so an in-flight turn is never interrupted; the next process
   continues the same Goal on its next interval.
5. **Rollback on failure**: the new version marks the switch `healthy` itself; if
   the shell cannot reach its service within the grace period (10 minutes), it puts
   `current.json` back on the previous version and restarts that version's inner
   binary (3 / 15 minute budget).
6. **Installed versions are never deleted or overwritten** (matching the SOP's
   prohibition): only a pointer and a marker are added.

## 10. OS watchdog (restores the app after the tree died or the machine rebooted)

A process that is gone cannot restart itself, so this layer needs an OS resident. The
watchdog is a **pre-shell mode** of the host binary, called on a schedule:

```bash
reasonix-desktop.exe --watchdog          # the scheduler calls this every 5 minutes
reasonix-desktop.exe --watchdog-status   # policy / registered on this machine / entry point
reasonix-desktop.exe --watchdog-enable   # write the policy and register at once
reasonix-desktop.exe --watchdog-disable  # write it, unregister, remove the desktop file
```

- **Driver: the master switch owns this entry.** Turning the switch on registers the
  watchdog and writes the desktop file; turning it off unregisters the task and removes
  the file, so nobody has to remember a second command. The four commands above remain
  for inspection and for a manual override, and every start realigns the entry with the
  stored switch (a switch written by an older host, or a hand-edited policy, converges).
  A host that is not a versioned install (a dev run) never touches the OS entry.
- **One criterion**: a marker exists ∧ `unattended=true` ∧ its process is gone ∧ the
  crash is under 24 hours old → launch the active version's inner desktop binary. A
  clean exit removes the marker, so a deliberate quit is never undone.
- **Registration**: a Windows scheduled task `ReasonixDesktopWatchdog` (every 5
  minutes), or `~/Library/LaunchAgents/io.reasonix.desktop.watchdog.plist` (RunAtLoad
  + StartInterval 300). Both point at **that script only, never at a version**.
- **Policy**: `<home>/desktop-autostart.json` (`enabled` + `watchdog`, defaulting to the
  policy's `enabled`), which also drives the Electron login item.
- **The one management file**: `C:\Users\guosj\Desktop\$\watchdog.cmd` — the watchdog's
  visible twin: run it by hand, read it, or delete it. It makes **no version decision**:
  it starts the **stable launcher** with `REASONIX_WATCHDOG=1` in the environment, and
  that launcher resolves `current.json` on every run — so after a build or a version
  switch the watchdog necessarily brings up the **newest** version, without rewriting
  the file. The app still rewrites it on every start and toggle (its paths can change)
  and removes it when the watchdog is disabled. That directory holds only this file;
  the inspection trail goes to `<home>/desktop-watchdog.log` (one line
  per run: time, action, reason).

## 11. Limits and open items

- The old session's `scopeID`, DeliveryCheckpoint and todos are **not** migrated:
  the new session re-anchors by the same `goal` text, so the delivery-evidence
  chain loses one link.
- macOS uses a LaunchAgent **sweep** (RunAtLoad + StartInterval), not `KeepAlive`; an
  Electron main crash that exhausts the relaunch budget parks on the failure page
  (deliberate — never loop) and the next sweep brings it back.
- The panel does not display the master switch's current value yet. The watchdog has no
  toggle of its own: the master switch drives it (§10), and the CLI stays for inspection.
  Neither the watchdog nor the panel shows why a registration was refused — that lands in
  `<home>/desktop-watchdog.log` and the host log.
- Neither full chain has been accepted on a real machine yet: neither kill the
  process abnormally → relaunch → the long task continues, nor build → switch
  versions → continue (both are unit-verified only).
