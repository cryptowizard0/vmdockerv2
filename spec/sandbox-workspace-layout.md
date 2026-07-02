# Sandbox Workspace Layout

This document defines the runtime workspace contract for `vmdocker` process instances created under:

- `/Users/webbergao/work/src/HymxWorkspace/vmdocker/cmd/sandbox_workspace/<pid>`

It applies to both Docker and Docker Sandbox backends unless a specific backend documents a stricter rule.

## Purpose

Each spawned process gets one isolated writable workspace root:

- `<workspace-root> = .../sandbox_workspace/<pid>`

`vmdocker` persists runtime state, agent state, caches, and temporary files under this root so that:

1. checkpoint and restore can archive a single directory tree,
2. writable state is scoped to one process instance,
3. the container root filesystem can stay read-only.

## Directory Contract

The runtime workspace layout is created by `vmdocker/runtimemanager/env.go`.

Expected directories:

```text
<workspace-root>/
├── workspace/
├── .home/
├── .tmp/
├── .xdg/
│   ├── config/
│   ├── cache/
│   └── state/
└── .openclaw/
    └── workspace/
```

### `workspace/`

- Mapped to `VMDOCKER_AGENT_WORKSPACE`
- Primary working directory for the active agent runtime
- For Claude runtime, this is the intended cwd for agent file operations

### `.home/`

- Mapped to `HOME` and `VMDOCKER_RUNTIME_HOME`
- Holds user-scoped tool state and config
- For Claude runtime, this commonly contains:
  - `.claude.json`
  - `.claude/`
  - session logs
  - project memory

### `.tmp/`

- Mapped to `TMPDIR`
- Temporary file area for runtime processes
- Kept under the workspace root so temp state survives checkpoint/restore when needed

### `.xdg/config`, `.xdg/cache`, `.xdg/state`

- Mapped to:
  - `XDG_CONFIG_HOME`
  - `XDG_CACHE_HOME`
  - `XDG_STATE_HOME`
- Prevent tools from writing outside the runtime workspace through default XDG paths

### `.openclaw/`

- Backward-compatible state area for OpenClaw-oriented runtime layout
- Still created for unified workspace layout even when the active runtime is Claude
- Not the primary Claude workspace

## Permission Contract

The runtime container is expected to run as a non-root user:

- Current images use `agent`

The container root filesystem is expected to be read-only:

- `ReadonlyRootfs=true`

The runtime workspace bind mount is expected to be the primary writable area for normal process execution.

In practice this means:

- `/` is read-only for the runtime user
- `/app` is read-only at runtime even if owned by `agent`
- `/tmp` should not be relied on as writable application storage
- `<workspace-root>` and its managed subdirectories are the intended writable locations

## Ownership Expectations

On the host, ownership may appear as the host user that created the workspace directory.

Inside the container, the same mounted path may resolve to the runtime user name, typically:

- `agent:agent`

This is expected and usually reflects UID/GID mapping rather than a mismatch.

## Writable Scope Rule

Agents running inside `vmdocker` should treat the following as writable-by-design:

- `<workspace-root>/workspace`
- `<workspace-root>/.home`
- `<workspace-root>/.tmp`
- `<workspace-root>/.xdg`
- `<workspace-root>/.openclaw`

Agents should treat other filesystem locations as effectively read-only unless a runtime explicitly provisions additional writable mounts.

## Operational Guidance

When debugging runtime behavior:

1. Inspect the instance root under `cmd/sandbox_workspace/<pid>`
2. Check which env vars point into that tree
3. Confirm the container user is non-root
4. Confirm the container root filesystem is read-only
5. Confirm writes are landing only under the workspace root

## Code References

- `/Users/webbergao/work/src/HymxWorkspace/vmdocker/vmdocker/runtimemanager/env.go`
- `/Users/webbergao/work/src/HymxWorkspace/vmdocker/vmdocker/runtimemanager/sandbox_test.go`
- `/Users/webbergao/work/src/HymxWorkspace/vmdocker/vmdocker/runtimemanager/env_test.go`
