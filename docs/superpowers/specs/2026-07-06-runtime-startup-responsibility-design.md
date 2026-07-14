# Runtime Startup Responsibility Redesign

- **Status:** Draft (partially superseded — see below)
- **Superseded in part by:** [ADR-0001 — profile `startup` → literal `CMD`](../../adr/0001-startup-to-cmd.md). The `[dockerfile].startup` → `user-startup.sh` COPY mechanism (§6.3, §8.1) is replaced by an inline `[dockerfile].CMD`; the rest of this design (adapter as PID 1 init, host-enforced confinement, health-gated readiness) still stands.
- **Date:** 2026-07-06
- **Owner:** vmdockerv2 / vmdocker_agent
- **Repos in scope:** `vmdockerv2` (host + modulebuild), `vmdocker_agent` (adapter + startup scripts)
- **Supersedes:** the `start-vmdocker-agent.sh` wrapper + `bootstrap/*.sh` + the wrapper's `RUNTIME_TYPE`-based bootstrap dispatch (the `RUNTIME_TYPE` env itself is kept — see §2)

## 1. Background

A spawned runtime container today boots through a platform-injected shell
ENTRYPOINT, `start-vmdocker-agent.sh`, which performs an in-container security
audit, dispatches a per-runtime bootstrap hook (`bootstrap/openclaw.sh`,
`bootstrap/claude.sh`) that starts the runtime engine and waits for it to become
ready, runs an untrusted user startup hook under a timeout, and finally `exec`s
the Go adapter that serves the platform API on port 8080.

Two problems drove this redesign:

1. **The in-container checks are not a security boundary.** The container image
   bytes come from the module payload (`Image-Source: module-data`), so the
   ENTRYPOINT script, the bootstrap hooks, and every check in them live inside
   the module author's control domain. A malicious author edits or removes them
   at will. Real confinement can only come from the host (kernel + Docker
   daemon) and is already largely enforced in `buildDockerHostConfig`.

2. **Engine bring-up is the module author's concern, not the platform's.** The
   author already declares the runtime by choosing the `FROM` alias, and knows
   how their engine must be started. Keeping that logic in a platform-owned
   shell wrapper (via `RUNTIME_TYPE` dispatch to `bootstrap/*.sh`) is misplaced
   ownership and dead weight.

## 2. Goals

- Move runtime engine bring-up out of the platform wrapper and into a
  module-author-owned `start.sh`, which becomes the primary customization point.
- Make the host the single authority for confinement (enforcement, not checks)
  and for readiness (authoritative health polling, not container self-report).
- Delete non-essential layers: the shell wrapper, `bootstrap/*.sh`, the
  wrapper's `RUNTIME_TYPE`-based bootstrap dispatch, the in-container security
  audit, and the shell-level health-wait loop.
- Simplify failure semantics to a single health-gated model.

> **Note — `RUNTIME_TYPE` stays.** The `RUNTIME_TYPE` env is *not* removed. It is
> load-bearing for the adapter: `runtime.newRuntime` reads it to select the
> runtime backend (openclaw / claude / telegramcustomer / test) when
> `/vmm/spawn` is called. Only the *wrapper's* use of it — dispatching to
> `bootstrap/<type>.sh` — goes away, because the wrapper and those hooks are
> deleted. The generated Dockerfile keeps `ENV RUNTIME_TYPE`.

## 3. Non-Goals

- Scanning or validating module image contents on the host (sudoers, entrypoint
  hashes). Checks are bypassable; the host enforces via mechanism instead.
- Changing the module packaging format or `ModuleFormat`
  (`hymx.vmdockerv2.v0.0.1`).
- Redesigning the Docker Sandbox backend confinement (platform-enforced,
  tracked separately).
- **Egress / outbound network hardening.** Full outbound remains as-is in this
  spec; see §11 Future Work.

## 4. Trust Model & Principle

Three parties in three trust domains. The governing rule:

> **What can be enforced by the kernel/daemon belongs to the host. What must be
> orchestrated in-container and cannot be delegated to the author belongs to the
> adapter. Author business belongs to `start.sh`. Responsibilities do not flow
> down across a trust boundary.**

| Layer | Trust | Enforcement power | Provided by |
|---|---|---|---|
| **host** (vmdockerv2) | fully trusted | kernel + Docker daemon (real) | platform |
| **adapter** (Go, PID 1) | platform-injected, runs inside attack surface | none for confinement; owns readiness | platform |
| **`start.sh`** | untrusted | none; constrained by host + adapter | module author |

## 5. Target Architecture

```
ENTRYPOINT = adapter (Go, PID 1); RUNTIME_TYPE still selects the backend

adapter on startup:
  1. run runtime-type-specific prep (openclaw: PrepareOpenclawRuntime) and
     export the resulting workspace-convention env
     (OPENCLAW_STATE_DIR / OPENCLAW_CONFIG_PATH / OPENCLAW_GATEWAY_LOG_PATH)
  2. spawn start.sh in the background; capture its stdout/stderr to a log
  3. immediately serve the platform API on :8080
  4. gate /vmm/health on runtime-type-specific readiness (see §6.2)
  5. act as PID 1 init: reap zombies, forward SIGTERM (graceful engine stop)

start.sh (module author; platform ships a default template per base image):
  - consume the env exported by the adapter
  - start the runtime engine (e.g. openclaw gateway) in the background
  - perform init / workspace seeding
  - return; the engine keeps running

host (vmdockerv2):
  - enforce isolation, resources, non-root (HostConfig)
  - node startup: one-shot daemon-capability self-check
  - poll /vmm/health until ready or timeout; authoritative readiness
```

Result: three shell/config layers collapse to **two platform entities + one
author script**. `start-vmdocker-agent.sh` and `bootstrap/` are removed;
`RUNTIME_TYPE` is retained (it drives adapter backend selection and per-type
readiness).

## 6. Responsibilities

### 6.1 host (vmdockerv2)

Enforce what checks cannot guarantee, in
`buildDockerHostConfig` / `buildDockerContainerConfig`:

- **Existing (keep):** `ReadonlyRootfs`, `CapDrop: ALL`,
  `no-new-privileges:true`, MaskedPaths/ReadonlyPaths, memory/CPU/PidsLimit,
  port bound to `127.0.0.1`, single writable per-pid workspace mount,
  authoritative `/vmm/health` polling in `waitForContainerReady`.
- **New — enforce non-root `User`:** set a fixed non-root uid on the container
  config so a `USER root` image cannot run as root. This replaces the
  in-container passwordless-sudo audit — it makes escalation impossible rather
  than checking that it did not happen. The enforced uid must match the
  non-root user the generated Dockerfile creates (`hymx`), so the workspace
  bind-mount ownership and this setting stay consistent.
- **New — disable swap dilution:** set `MemorySwap` equal to `Memory` (currently
  `-1` = unlimited swap), so the memory ceiling cannot be diluted by swap.
- **Network:** unchanged in this spec. No `NetworkMode` tightening, no egress
  filtering; full outbound remains. Egress hardening is deferred (§11).

#### Node-startup self-check

At vmdockerv2 node startup, run **one** `cli.Info()` call against the local
(trusted) Docker daemon and assert that the confinement the host config requests
will actually take effect. This is not an in-container check: it inspects the
platform's own daemon capabilities, so it is trustworthy, and it validates
"will my enforcement land" rather than "is there something bad in the
container".

| Check | Source (`cli.Info()`) | If absent | Default severity |
|---|---|---|---|
| daemon reachable + supported version | Ping / ServerVersion | cannot spawn at all | **refuse (fatal)** |
| seccomp default profile active | SecurityOptions contains `seccomp` (not unconfined) | syscall filtering off, cap-drop weakened | **refuse** (configurable) |
| memory limit supported | `Info.MemoryLimit == true` | HostConfig `Memory` silently ignored | **refuse** (configurable) |
| swap limit supported | `Info.SwapLimit == true` | `MemorySwap=Memory` cannot be enforced | warn |
| pids limit supported | `Info.PidsLimit == true` | fork-bomb limit ignored | warn |
| AppArmor or SELinux present | SecurityOptions contains `apparmor`/`selinux` | MAC defense-in-depth absent (cap-drop still holds) | warn |
| cgroup version / driver | `Info.CgroupVersion` / `CgroupDriver` | resource accounting less reliable | warn |
| rootless / userns status | SecurityOptions | posture signal (rootful daemon?) | info |

The check emits a structured pass/warn/fail report. `warn` vs `refuse` is
configurable; the defaults above follow the rule "if the gap turns a host
enforcement into an empty promise, it is `refuse`; if it only weakens
defense-in-depth while the primary enforcement still holds, it is `warn`". A
strict/production mode may escalate all `warn` to `refuse`.

host does **not**: inspect image contents.

### 6.2 adapter (Go, PID 1) — vmdocker_agent

Absorbs the former wrapper. The adapter is the trusted supervisor of the
untrusted `start.sh`. The adapter still selects its backend from `RUNTIME_TYPE`
(`runtime.newRuntime`), so the steps below are runtime-type-aware:

- **Runtime-type-specific prep + env export.** Before spawning `start.sh`, run
  the prep for the selected `RUNTIME_TYPE` and export the resulting
  workspace-convention env so the default template consumes fixed paths instead
  of recomputing them. For **openclaw** this is `utils.PrepareOpenclawRuntime`
  (today invoked via `bootstrap prepare --shell`), exporting `OPENCLAW_STATE_DIR`
  / `OPENCLAW_CONFIG_PATH` / `OPENCLAW_GATEWAY_LOG_PATH`. For **claude** / **test**
  there is no engine prep; this step is a no-op.
- Spawn `start.sh` as a subprocess; do **not** block API serving on its
  completion.
- Capture `start.sh` stdout/stderr to a log path the host can retrieve.
- **Own `/vmm/health` with runtime-type-specific readiness.** The handler today
  (`server.(*Server).health`) returns `200` unconditionally; this changes to a
  readiness gate:
  - **openclaw:** ready only when the gateway is reachable (an
    `HTTPGatewayClient.Init` / ping to the gateway `/health` succeeds).
  - **claude:** ready when the `claude` CLI is present on `PATH` (the former
    `bootstrap/claude.sh` check).
  - **test:** always ready.
- **PID 1 init duties, implemented natively in Go:** a signal handler plus a
  `waitpid`/reap loop to reap zombie processes, and `SIGTERM` forwarding to
  `start.sh` / engine for graceful shutdown. No tini, no shell wrapper. (The
  server's existing `SIGINT`/`SIGTERM` handling in `Run` is extended, not
  replaced.)

The old isolation hack (child-process + `timeout` + failure-ignored) is deleted:
because the adapter is already serving independently of `start.sh`, a hung or
crashed `start.sh` cannot prevent the adapter from starting — it only keeps
health red, which the host observes.

### 6.3 `start.sh` — module author

- Consume the env the adapter exported, start the runtime engine (openclaw
  gateway, or whatever the module needs) in the background, and perform
  initialization / workspace seeding.
- Explicitly **not** the ENTRYPOINT and **not** part of any security or
  readiness decision.
- **The platform ships a default `start.sh` template per base image** (openclaw,
  claude). The engine-start logic lives visibly in the template — there is no
  hidden platform helper or sourced library. The author uses the template as-is,
  extends it, or copies and fully replaces it via `profile.toml`
  `[dockerfile].startup`.

## 7. Failure Semantics

A single health-gated model. The host's `/vmm/health` polling is the only
failure detector; the container never self-reports success authoritatively.

| Scenario | Outcome |
|---|---|
| `start.sh` exits non-zero (engine not up) | adapter serves; `/vmm/health` stays red; host times out with captured `start.sh` log |
| `start.sh` hangs / never returns | adapter already serving; `/vmm/health` stays red; host times out with log |
| `start.sh` behaves maliciously | contained by host (non-root, cap-drop, resource limits); adapter unaffected |
| engine ready per runtime type (§6.2) | `/vmm/health` → 200; host marks ready |

## 8. Changes by Repo

### 8.1 vmdockerv2

- `vmdocker/runtimemanager/docker.go`
  - `buildDockerContainerConfig`: set non-root `User`; ENTRYPOINT points at the
    adapter (no shell wrapper).
  - `buildDockerHostConfig`: `MemorySwap = Memory`. Network left unchanged.
- `vmdocker/modulebuild/dockerfile.go` + `dockerfile_test.go`
  - Remove `start-vmdocker-agent.sh` (`WrapperSrc`) injection; ENTRYPOINT =
    adapter binary (`/usr/local/bin/vmdocker-agent`). **Keep `ENV RUNTIME_TYPE`**
    (adapter backend selection). `[dockerfile].startup` still COPY'd to
    `/usr/local/lib/vmdocker-agent/user-startup.sh`.
  - Drop the `WrapperSrc` field from `DockerfileInput` / `dockerfileView` and the
    `WrapperSrc` required-arg check in `GenerateDockerfile`.
- `vmdocker/modulebuild/build.go`
  - Drop `BuildOptions.WrapperPath` and the wrapper staging in
    `stageBuildContext` (no more `platform/start-vmdocker-agent.sh`); keep staging
    `platform/vmdocker-agent`.
- `vmdocker/runtimemanager/docker.go`
  - `buildDockerContainerConfig`: set non-root `User` (aligned with `hymx`);
    ENTRYPOINT already comes from the image, no shell wrapper.
  - `buildDockerHostConfig`: `MemorySwap = Memory`. Network left unchanged.
- vmdockerv2 node startup: add the one-shot `cli.Info()` confinement self-check
  (§6.1), with configurable warn/refuse policy.

### 8.2 vmdocker_agent

- Adapter: run runtime-type-specific prep + export env; spawn + supervise
  `start.sh`; capture logs; gate `/vmm/health` on runtime-type-specific
  readiness (§6.2); implement PID 1 init natively in Go (reap + `SIGTERM`
  forwarding). **Keep `RUNTIME_TYPE` reads in `runtime.newRuntime`.**
- Delete `start-vmdocker-agent.sh`, `bootstrap/openclaw.sh`,
  `bootstrap/claude.sh`, and `scripts/test_startup_hook_isolation.sh`.
- Add default `start.sh` template for openclaw (start gateway using the exported
  env) and claude (no engine; init only).
- Update `Dockerfile.openclaw`, `Dockerfile.claude`, `Dockerfile.telegramcustomer`:
  ENTRYPOINT = adapter (`/app/main`), remove the wrapper COPY + ENTRYPOINT and the
  `bootstrap/` COPY, COPY the default `start.sh` template to
  `/usr/local/lib/vmdocker-agent/user-startup.sh`, keep `ENV RUNTIME_TYPE`.

## 9. Risks & Tradeoffs

- **Adapter as PID 1 init (decided: Go-native).** Go does not auto-reap or
  auto-forward signals as PID 1, so the adapter must implement a `SIGCHLD`/reap
  loop and `SIGTERM` forwarding itself. This is the real cost of replacing the
  shell wrapper; the payoff is testable Go supervision at the correct trust
  level. (tini / a 3-line shell shim were considered and rejected — they add a
  dependency or keep a shell in the ENTRYPOINT while the adapter still must
  reap.)
- **Engine-start logic duplicated into modules.** Because the default `start.sh`
  is a template the author copies, a change to how the engine should start (e.g.
  new openclaw gateway flags) does not automatically propagate to modules that
  forked the template. Accepted tradeoff for full author ownership and zero
  hidden platform behavior; mitigated by keeping the template minimal and
  documenting upgrades.
- **`profile.startup` semantics change** from "optional user hook" to "runtime
  bring-up script (default template provided, override-able)". Requires a
  doc/schema note and default templates so existing modules keep working.
- **ENTRYPOINT is now a Go binary**, less trivially inspectable/override-able
  than a shell script. Mitigated by the shipped default `start.sh` covering
  shell-level customization, and `docker run --entrypoint` for debugging.

## 10. Acceptance Criteria

1. A module built via modulebuild runs with adapter as ENTRYPOINT, no
   `start-vmdocker-agent.sh`, no `bootstrap/*.sh`. `ENV RUNTIME_TYPE` is still
   present and still selects the adapter backend.
2. Container runs as a fixed non-root uid even if the image declares
   `USER root`; passwordless sudo is impossible (mechanism, not check).
3. `MemorySwap == Memory` on the spawned container.
4. openclaw module: default `start.sh` template backgrounds the gateway using
   the env the adapter exported; host observes `/vmm/health` → ready only after
   the gateway ping succeeds. claude module: `/vmm/health` → ready when the
   `claude` CLI is on `PATH`. test: always ready.
5. A hanging or crashing `start.sh` never blocks adapter startup; host times out
   with the captured `start.sh` log available.
6. Adapter reaps zombies and forwards `SIGTERM` to the engine for graceful
   shutdown.
7. vmdockerv2 node startup runs the `cli.Info()` self-check once, emits a
   structured report, and refuses/warns per the configured policy (defaults per
   §6.1 table).
8. `vmdocker_agent` base Dockerfiles (`openclaw`, `claude`, `telegramcustomer`)
   launch the adapter directly as ENTRYPOINT with no wrapper.

## 11. Future Work

- **Egress / outbound hardening.** Out of scope here; full outbound remains. The
  intended future direction: per-module egress declaration in `profile.toml`
  (`[vmdocker.egress].allow = ["host:port", ...]`), container `default-deny`,
  and a platform-injected filtering proxy that enforces the declared domain
  allowlist (author declares, host enforces — consistent with §4). The proxy is
  a new platform component with its own design surface (where it runs, proxy
  software, SNI-filtering vs TLS interception, allowlist delivery at spawn) and
  will get its own spec.
