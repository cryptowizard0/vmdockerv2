# ADR-0001: Profile `startup` hook becomes a literal Dockerfile `CMD`

- **Status:** Accepted
- **Date:** 2026-07-14
- **Supersedes (in part):** [2026-07-06 runtime-startup-responsibility design](../superpowers/specs/2026-07-06-runtime-startup-responsibility-design.md) — its `[dockerfile].startup` → `user-startup.sh` COPY mechanism (§6.3, §8.1). The rest of that design (adapter as PID 1 init, host-enforced confinement, health-gated readiness) stands.

## Context

A module declares its container startup behavior through `[dockerfile].startup`, a **required** profile field pointing at a separate `start.sh` file. modulebuild COPYs that file into the image at a fixed path (`/usr/local/lib/vmdocker-agent/user-startup.sh`), and the adapter runs it as a backgrounded hook.

Two frictions drove this decision:

- **File indirection.** Even a one-line startup needs a whole extra file shipped next to `profile.toml`, and the field is required — so even a no-op module must author a placeholder script that does nothing.
- **Conceptual mismatch.** `startup = "start.sh"` reads as "a script path," but authors who think in Dockerfile terms expect to write the startup **command inline** — a command plus arguments — the way `ENTRYPOINT` / `CMD` read.

## Decision

Replace `[dockerfile].startup` (required, a file path) with `[dockerfile].CMD` (optional, an inline command), using Dockerfile `CMD` syntax:

- **exec form** — a TOML array: `CMD = ["node", "init.js", "--seed"]`
- **shell form** — a bare string: `CMD = "node init.js --seed"` (wrapped by `/bin/sh -c`, like Dockerfile shell form)

modulebuild bakes this into the generated image as a real Dockerfile `CMD` line (array → exec form, string → shell form); when `CMD` is unset it emits **no** `CMD` line. The platform adapter (`vmdocker-agent`) stays the container's real `ENTRYPOINT` / PID 1 init; the author's `CMD` is the payload it launches and supervises. This is Docker's canonical **init-wrapper pattern**: `ENTRYPOINT` = a tini/dumb-init-style supervisor (here, the adapter), `CMD` = the command it runs.

### Why `CMD` and not `ENTRYPOINT`

The field was almost named `ENTRYPOINT`. That was rejected: the value **does not** become the image's `ENTRYPOINT` — the adapter does. Naming it `ENTRYPOINT` would be a name that lies, and it would collide with the retained invariant *"the author command must never become the container `ENTRYPOINT`"*, which keeps the adapter's role as trusted PID 1 from regressing. `CMD` is the honest Dockerfile analogue. Making the author command the real PID 1 (bypassing the adapter) was likewise rejected — it would forfeit the adapter's health/readiness gating, checkpoint/restore, and sandbox hardening.

## Consequences

- **Single source of truth.** The startup command lives **only** in the image's baked `CMD` (`Config.Cmd`), consumed by both runtime backends. The docker backend leaves `Entrypoint`/`Cmd` unset on the container config, so the image's baked `ENTRYPOINT` (adapter) + `CMD` (author command) apply. The sandbox backend derives its launch command from the image config it already inspects (`Config.Entrypoint` + `Config.Cmd`) and composes `<entrypoint> <cmd>`. The per-spawn `Start-Command` override is removed from the default path so `CMD` cannot be silently emptied at spawn.
- **Cross-repo change in `vmdocker_agent`.** The adapter reads its command from its **process arguments** (everything after `argv[0]`) instead of executing a fixed on-disk script path. Given no arguments it falls back to the existing `user-startup.sh` hook path, so base images that still ship a default startup script keep working during the transition. The adapter argv change and this build change must land together, verified by the shared real-build-spawn e2e.
- **`CMD` is optional.** Trivial modules omit it — there is no more placeholder `start.sh`. The build no longer COPYs a user startup script into the image, so the build context no longer stages a user-provided script.
- **Out of scope.** A `CMD` is a single command (exec form) or one shell line (shell form). Multi-step bring-up ("start the engine in the background, seed, then return") is not expressible inline; such modules keep that logic in a script the base image ships or in build-time `RUN`.
