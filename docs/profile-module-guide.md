# VMDocker V2 Guide: profile.toml → Module End to End

> For developers who want to author, build, load and spawn a sandbox Module in vmdockerv2.
> Every command/snippet below maps to the current implementation and can be run as-is.
> Key sources: [`vmdocker/modulebuild/`](../vmdocker/modulebuild/), [`cmd/module/`](../cmd/module/), [`vmdocker/module_image.go`](../vmdocker/module_image.go), [`vmdocker/capability/`](../vmdocker/capability/).

---

## 0. TL;DR

```bash
# 1) Build the platform adapter binary (linux vmdocker-agent, from the sibling repo vmdocker_agent)
cd ../vmdocker_agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/vmdocker-agent .

# 2) Write a profile.toml (see §2), next to its bin/ dir and startup script
#    mymod/{profile.toml, bin/, start.sh}

# 3) Build and sign it into a module (see §3)
cd ../vmdockerv2
export VMDOCKER_URL=http://127.0.0.1:8080
export VMDOCKER_PRIVATE_KEY=0x<your-key>
go run ./cmd/module -profile ./mymod/profile.toml -agent-bin /tmp/vmdocker-agent
#  -> [module] saved module <id> -> mod-<id>.json

# 4) Make the node see it, then spawn (see §4)
cp mod-<id>.json ./mod/mod-<id>.json      # local testing: drop it into the node's mod/ dir
./build/hymx-node --config ./config.yaml  # start the node; it loads the image by module id on spawn
```

---

## 1. New architecture in brief

vmdockerv2 is the Docker-backed sandbox VM for the HyMatrix compute network. Compared to V1, V2 replaces the old **scattered tags + build-at-spawn** model with a **single declarative `profile.toml` that drives a self-contained image module**. The module format is the single constant `hymx.vmdockerv2.v0.0.1` (see [`vmdocker/runtimemanager/schema/schema.go`](../vmdocker/runtimemanager/schema/schema.go)).

Core ideas:

1. **Declarative, single input.** You only write `profile.toml` (pick a base alias, drop in a few executables, one startup script, optional packages). The builder **generates a standardized, hardened Dockerfile** from it — you never hand-write a Dockerfile. Hardening includes: a fixed container `ENTRYPOINT` (the `vmdocker-agent` adapter), a non-privileged `hymx` user, removal from the `sudo`/`docker` groups, a read-only root filesystem, and so on.

2. **The image travels with the module (self-contained).** The build does `docker save | gzip` into `image.tar.gz` and packs it — together with `profile.toml` (and an optional `public.zip`) — into one **container-tar** that is the module payload. On spawn, if the local Docker has no matching image, it is restored from the module payload via `docker image load` — there is **no build at spawn time**. Archive-format constant: `container-tar+image.tar.gz`.

3. **The module only describes the image; how it runs is decided at spawn.** `Runtime-Backend` (`docker`/`sandbox`) and `Start-Command` come from spawn tags; if omitted they default by OS (Linux → `docker`, macOS/Windows → `sandbox`).

4. **A fixed runtime workspace contract.** Each instance workspace is `<root>/sandbox_workspace/<pid>`, mapped inside the container to `HOME=/home/hymx`, with `OPENCLAW_*`, `XDG_*`, `TMPDIR`, etc. pre-seeded. Both backends write runtime state only inside that mapped workspace.

5. **Public state travels in `public.zip`.** The `[vmdocker].public` allowlist in `profile.toml` declares which HOME-relative files are public. It is collected into a `public.zip` in two places: at **build** time `cmd/module` collects the files from the profile directory (so a fresh module ships its authored initial state — skills, persona, ...), and at **Export** time the running agent's current files are collected from its live workspace (so a clone carries evolved state). Either way `public.zip` is packed into the module and seeded into a fresh workspace on spawn. There is no Import.

### Data-flow overview

```
 profile.toml + bin/ + startup + platform/vmdocker-agent
        │   modulebuild.BuildModuleArtifact  (cmd/module)
        ▼
 generate standardized Dockerfile ─► docker build ─► docker save|gzip ─► image.tar.gz
        │
        ▼  PackModule: tar{ image.tar.gz + profile.toml [+ public.zip] } | gzip
   ModuleArtifact{ ModuleBytes, Tags }
        │   sign via hymx SDK (goether key + goar bundler)
        ▼
   mod-<id>.json  ──►  drop into the node's mod/ dir (or auto-cached after a network download)
        │   vmdocker.Spawn → (*VmDocker).Run
        ▼
   ensureModuleImageAvailable → docker image load when the image is missing
   CreateInstance            → workspace = sandbox_workspace/<pid>
   seedWorkspaceFromModule   → write profile.toml + unpack public.zip into the workspace
   StartInstance             → container ENTRYPOINT=vmdocker-agent → runs user-startup.sh → ready
```

---

## 2. How to write profile.toml

The schema is defined in [`vmdocker/modulebuild/profile.go`](../vmdocker/modulebuild/profile.go) and has exactly two tables: `[dockerfile]` and `[vmdocker]`.

### 2.1 Field reference

**`[dockerfile]` — how the image is built**

| Key | Required | Type | Meaning |
|---|---|---|---|
| `FROM` | ✅ | string | Base image **alias** (not a real image name), resolved by `ResolveFROM`. Supported: `openclaw`, `claude` |
| `bin` | ✅ | string | A directory name; its executables are `COPY {bin}/ → /usr/local/bin/` |
| `startup` | ✅ | string | Your startup script; `COPY → /usr/local/lib/vmdocker-agent/user-startup.sh`. **It is your hook, not the container ENTRYPOINT** |
| `tools` | ⬜ | []string | System packages to install; the build auto-detects `apt-get`/`apk`/`microdnf` |
| `RUN` | ⬜ | []string | Extra Dockerfile `RUN` instructions **without the `RUN` prefix**; each renders as one line |

> `FROM` alias map (see [`vmdocker/modulebuild/dockerfile.go`](../vmdocker/modulebuild/dockerfile.go) `baseAliases`):
> | Alias | Actual base image | `RUNTIME_TYPE` |
> |---|---|---|
> | `openclaw` | `docker/sandbox-templates:shell` | `openclaw` |
> | `claude` | `docker/sandbox-templates:claude-code` | `claude` |
> An unknown alias fails fast: `unknown FROM alias ...`.

**`[vmdocker]` — which files may be exported**

| Key | Required | Type | Meaning |
|---|---|---|---|
| `public` | ⬜ | []string | Public path allowlist, collected into `public.zip` at build (from the profile dir) and at Export (from the live workspace), then seeded on spawn. Each entry **must start with `~/`** and is a HOME-relative glob |

`public` matching rules (see [`vmdocker/capability/publicmatch.go`](../vmdocker/capability/publicmatch.go)):

- `*` matches any run of characters **including `/`** (i.e. recursive, cross-directory); `?` matches one character; everything else is literal.
- Must be `~/`-prefixed, otherwise the entry is skipped (fail-closed: a skipped entry allows nothing).
- No `..` segments.
- **No HOME-root-level globs** (e.g. `~/*`, `~/*.md`) — they would sweep in `.ssh`/`.bashrc` and other dotfiles; too broad, rejected. Scope them to a subdirectory.
- A non-glob entry pointing at a directory is a usage error (it hints to use `~/dir/*`).
- A symlink resolving outside HOME is rejected (`PATH_ESCAPE`).

Example:

```toml
public = [
  "~/investment.md",   # exact single file
  "~/skills/*",        # every file under skills/ (recursively)
  "~/persona/*.md",    # every *.md under persona/ (recursively)
]
```

> Validation timing: `ParseProfile` only enforces that `[dockerfile].FROM` is non-empty; the non-empty checks for `bin`/`startup` happen when the Dockerfile is generated (`GenerateDockerfile`). So the **three required keys are `FROM`, `bin`, `startup`**.

### 2.2 Directory layout

The directory holding `profile.toml` (the build-time `ProfileDir`) must also carry the `bin` dir and the `startup` script:

```
mymod/
├── profile.toml
├── bin/                 # the [dockerfile].bin dir; put executables here
│   └── .keep            # may be empty, but the dir must exist
├── start.sh             # the [dockerfile].startup script
└── skills/              # optional: content declared by [vmdocker].public
    └── soul.md
```

### 2.3 A minimal, directly buildable example

From the real e2e fixture in [`vmdocker/realspawn_e2e_test.go`](../vmdocker/realspawn_e2e_test.go):

```toml
# mymod/profile.toml
[dockerfile]
FROM = "claude"
bin = "bin"
startup = "start.sh"

[vmdocker]
public = ["~/skills/*"]
```

```sh
# mymod/start.sh — your startup hook (claude readiness is CLI-on-PATH, so a no-op is fine)
#!/bin/sh
exit 0
```

A more practical example (install tools + custom RUN):

```toml
[dockerfile]
FROM = "openclaw"
bin = "bin"
startup = "start.sh"
tools = ["git", "jq", "ripgrep"]
RUN = [
  "mkdir -p /home/hymx/skills",
  "echo 'ready' > /etc/vmdocker-ready",
]

[vmdocker]
public = ["~/skills/*", "~/persona/*.md"]
```

### 2.4 The standardized Dockerfile generated for you

You never write it, but knowing it helps debugging. The template lives in [`dockerfile.go`](../vmdocker/modulebuild/dockerfile.go); the first profile in §2.3 renders roughly to:

```dockerfile
FROM docker/sandbox-templates:claude-code
USER root
WORKDIR /app

COPY platform/vmdocker-agent /usr/local/bin/vmdocker-agent
COPY bin/ /usr/local/bin/
COPY start.sh /usr/local/lib/vmdocker-agent/user-startup.sh
COPY profile.toml /home/hymx/profile.toml
RUN set -eux; \
    useradd --create-home --home-dir /home/hymx --shell /bin/bash hymx || true; \
    gpasswd -d hymx sudo || true; \
    gpasswd -d hymx docker || true; \
    rm -f /etc/sudoers.d/*
RUN set -eux; \
    chmod +x /usr/local/bin/* /usr/local/lib/vmdocker-agent/user-startup.sh; \
    chown -R hymx:hymx /home/hymx /app
ENV HOME=/home/hymx
ENV RUNTIME_TYPE=claude
USER hymx
WORKDIR /home/hymx
ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]
```

- With `tools` set, an adaptive `apt-get`/`apk`/`microdnf` install `RUN` is inserted in the middle.
- With `RUN` set, each entry renders as its own `RUN <your command>` line.
- The container entrypoint is always `vmdocker-agent` (the platform adapter); your `start.sh` is invoked by the adapter as `user-startup.sh`.

---

## 3. How to build a Module from profile.toml

The CLI entry point is [`cmd/module/main.go`](../cmd/module/main.go); it wires "build the artifact" to "sign and save it".

### 3.1 Prerequisite: the platform adapter binary (`-agent-bin`)

The container `ENTRYPOINT` injected into the module is the platform adapter `vmdocker-agent`, which comes from the **sibling repo `vmdocker_agent`**. Since it runs inside a linux container, cross-compile it for linux (see `resolveAdapterBinary` in `realspawn_e2e_test.go`):

```bash
cd ../vmdocker_agent        # default location: ../vmdocker_agent or ../../vmdocker_agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/vmdocker-agent .
```

Use `GOARCH=arm64` if the Docker host that will run the container is arm64. Then pass it via `-agent-bin /tmp/vmdocker-agent` or the env var `VMDOCKER_AGENT_BIN=/tmp/vmdocker-agent`.

### 3.2 Environment variables

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `VMDOCKER_PRIVATE_KEY` | ✅ | — | Ethereum private key (hex) used to sign the module |
| `VMDOCKER_URL` | ⬜ | `http://127.0.0.1:8080` | hymx SDK endpoint used to save/upload the module |
| `VMDOCKER_AGENT_BIN` | ⬜ | — | Env-var equivalent of `-agent-bin` |

### 3.3 Build command

```bash
cd /path/to/vmdockerv2
export VMDOCKER_URL=http://127.0.0.1:8080
export VMDOCKER_PRIVATE_KEY=0x64dd2342616f385f3e8157cf7246cf394217e13e8f91b7d208e9f8b60e25ed1b

go run ./cmd/module \
  -profile ./mymod/profile.toml \
  -agent-bin /tmp/vmdocker-agent
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-profile` | `profile.toml` | Path to profile.toml; its directory is the `ProfileDir` (`bin/`, `startup` are read from here) |
| `-agent-bin` | `$VMDOCKER_AGENT_BIN` | Path to the platform adapter binary, **required** |

### 3.4 What happens internally (`BuildModuleArtifact`)

See [`vmdocker/modulebuild/build.go`](../vmdocker/modulebuild/build.go) and [`module.go`](../vmdocker/modulebuild/module.go):

1. **Stage the build context** — `ParseProfile` → `GenerateDockerfile`; create a temp dir, write `Dockerfile` and `profile.toml`, copy `bin/`, `startup`, and `platform/vmdocker-agent`.
2. **`docker build`** — the tag defaults to `vmdocker-module:<hash>` where `<hash>` is the first 12 hex of the Dockerfile's sha256.
3. **Inspect the image id.**
4. **`docker save | gzip`** → `image.tar.gz`.
5. **Collect public + `PackModule`** — `cmd/module` collects the profile dir's `[vmdocker].public` files into a `public.zip` (via `capability.CollectPublic` + `BuildPublicZip`); `PackModule` assembles a container-tar of `image.tar.gz` + `profile.toml` + `public.zip`, gzips it into `ModuleBytes`, and emits the extension tags.
6. **Sign and save** — `cmd/module` uses a goether key + goar bundler through the hymx SDK `SaveModule`, producing `mod-<id>.json`.

Module tags emitted by `PackModule`:

| Tag | Value / example |
|---|---|
| `Image-Name` | build tag, e.g. `vmdocker-module:ab12cd34ef56` |
| `Image-ID` | `sha256:...` (image digest) |
| `Image-Source` | `module-data` (the image lives in the module payload) |
| `Image-Archive-Format` | `container-tar+image.tar.gz` |
| `Capability-Public` | comma-joined `profile [vmdocker].public` |
| `Created-At` | RFC3339 timestamp |
| `Member-Image-SHA256` | sha256 of the `image.tar.gz` member |

### 3.5 Expected output

```text
[module] building module artifact from profile
[module] artifact ready: tags=7 payload=<N> bytes
[module] saved module <generated-id> -> mod-<generated-id>.json
```

Note the `<generated-id>` — you need it to spawn.

---

## 4. Loading and using a Module

### 4.1 Make the module file available to the node

The runtime resolves the module file by id (see [`vmdocker/module_image.go`](../vmdocker/module_image.go) `resolveModuleFilePath`), relative to the node's working directory, trying in order:

1. `mod/mod-<id>.json` (preferred)
2. `mod-<id>.json` (legacy path)

For local testing just copy it in:

```bash
mkdir -p ./mod
cp mod-<id>.json ./mod/mod-<id>.json
```

> Over the network: after downloading a module from HyMatrix, the node auto-caches the same bundle as `mod/mod-<id>.json`, so no manual copy is needed.

### 4.2 Start the node

```bash
cd /path/to/vmdockerv2
go build -o ./build/hymx-node ./cmd
./build/hymx-node --config ./config.yaml
```

See [Configuration in the README](../README.md) for `config.yaml` (`port`/`redisURL`/`prvKey`/`joinNetwork`, ...). The node mounts the spawn handler under the module format via `s.Mount(ModuleFormat, vmdocker.Spawn)`.

### 4.3 Spawn: the load path

A spawn goes through `vmdocker.Spawn(env)` → `(*VmDocker).Run` (see [`vmdocker/vmdocker.go`](../vmdocker/vmdocker.go)):

1. `RuntimeSpecFromModuleAndSpawnTags(moduleFormat, moduleTags, spawnTags)` — merge module tags with spawn tags into a `RuntimeSpec` (backend, image, sandbox, start command).
2. `GetRuntimeManager(backend)`.
3. **`ensureModuleImageAvailable`** — if a local image matches `Image-Name` and the SHA, use it; otherwise extract `image.tar.gz` from the `data` field of `mod-<id>.json` (base64url → gzip → container-tar), `docker image load`, then re-tag by id and verify.
4. `CreateInstance` — create the instance; workspace = `sandbox_workspace/<pid>`.
5. **`seedWorkspaceFromModule`** — write the module's `profile.toml` into the workspace; if the module carries a `public.zip`, unpack it into the workspace with `capability.UnpackPublicZip` (path-safety checked).
6. `StartInstance` → `waitForContainerReady` → send the AO spawn request to the runtime.

Inside the container: `HOME=/home/hymx`, and seeded public content shows up at e.g. `/home/hymx/skills/soul.md`.

### 4.4 Spawn tags: decide how this run behaves

`RuntimeSpec`-related tags (see [`schema.go`](../vmdocker/runtimemanager/schema/schema.go)):

| Tag | Meaning |
|---|---|
| `Runtime-Backend` | `docker` or `sandbox`; if unset, defaults by OS: Linux → `docker`, macOS/Windows → `sandbox` (Linux rejects `sandbox`) |
| `Start-Command` | override the default start command (parsed as `command + args`, not a shell fragment) |
| `Sandbox-Agent` / `Sandbox-Network` / `Sandbox-Name` / `Sandbox-Command` | sandbox-backend details (`Sandbox-Agent` defaults to `shell`) |

> In production these tags — along with `provider`/`model`/`apiKey` etc. — are carried by the upper layer (HyMatrix SDK / the samples under `examples/`) when it sends the spawn message. See [`examples/`](../examples/).

### 4.5 Local end-to-end validation (no chain required)

The repo ships a host-side capability driver [`cmd/vmme2e`](../cmd/vmme2e/) and a script [`scripts/e2e_capability.sh`](../scripts/e2e_capability.sh):

```bash
# Part A (no docker): pack a synthetic module → run the real seed → assert profile+public land in a fresh workspace
# Part B (needs docker): mount the seeded workspace into a real container and assert it is visible
bash scripts/e2e_capability.sh

# Part C (heavyweight, needs docker + a pullable base image + the adapter binary):
# real docker build → real signed module → real in-process vmdocker.Spawn
RUN_REAL_SPAWN=1 bash scripts/e2e_capability.sh
```

`vmme2e` subcommands (all call production code directly, no new logic):

| Subcommand | Purpose |
|---|---|
| `vmme2e seed --module-id <id> --workspace <ws>` | seed only `profile.toml` into the workspace |
| `vmme2e seed-clone --module-id <id> --workspace <ws>` | seed `profile.toml` + `public.zip` (clone seed) |
| `vmme2e export --workspace <ws> --out <mod.json> [--agent-bin ..] [--signer-key ..]` | from a workspace containing `profile.toml`, collect public → rebuild image → pack and sign a new module |
| `vmme2e pack-synthetic --profile <p> --public-dir <d> --out <mod.json>` | build a synthetic module without docker (for crafting test cases) |

> Module id resolution matches the runtime: `mod/mod-<id>.json` relative to the current working directory.

### 4.6 Replication = spawn-a-clone (Export)

A running instance can export its public state into a new module — see [`vmdocker/capability/export.go`](../vmdocker/capability/export.go) `Export`:

1. Read `profile.toml` from the instance `HOME`.
2. Collect files by the `[vmdocker].public` allowlist via `CollectPublic` + `BuildPublicZip` into a `public.zip`.
3. `BuildModuleArtifact` rebuilds the image and packs it (`image.tar.gz` + `profile.toml` + `public.zip`).
4. `SignModuleArtifact` signs it into a new `mod-<id>.json`.

Spawn that new module through §4.1–§4.3 and you get a clone carrying the original agent's public content.

### 4.7 Verify cold start (restore the image from module data)

To confirm "the image can be restored from the module payload when it is absent locally":

```bash
docker image rm <Image-Name>     # remove the local image
# Spawn again with the same module id; it should still start:
# module file → data → gunzip → take image.tar.gz → docker image load → start
```

---

## 5. Cheat sheet

**Three required keys (profile.toml):** `[dockerfile].FROM` + `[dockerfile].bin` + `[dockerfile].startup`.

**Supported FROM aliases:** `openclaw` (shell template), `claude` (claude-code template).

**Module format constant:** `hymx.vmdockerv2.v0.0.1`; **archive format:** `container-tar+image.tar.gz` (loader also accepts the legacy `docker-save+gzip`); **image source:** `module-data`.

**Module file location:** `mod/mod-<id>.json` (or legacy `mod-<id>.json`), relative to the node working directory.

**public allowlist:** every entry `~/`-prefixed, HOME-relative glob (`*` recursive, `?` single char); no `..`, no HOME-root-level glob, no symlink escape.

**Build command:** `go run ./cmd/module -profile <p> -agent-bin <bin>` (needs `VMDOCKER_PRIVATE_KEY`).

### Key source index

| Topic | File |
|---|---|
| profile.toml struct & parse | [`vmdocker/modulebuild/profile.go`](../vmdocker/modulebuild/profile.go) |
| FROM aliases & Dockerfile template | [`vmdocker/modulebuild/dockerfile.go`](../vmdocker/modulebuild/dockerfile.go) |
| Build flow | [`vmdocker/modulebuild/build.go`](../vmdocker/modulebuild/build.go) |
| Pack & tags | [`vmdocker/modulebuild/module.go`](../vmdocker/modulebuild/module.go) |
| Build CLI entry | [`cmd/module/main.go`](../cmd/module/main.go) |
| Load (docker load) & seed | [`vmdocker/module_image.go`](../vmdocker/module_image.go) |
| Spawn flow | [`vmdocker/vmdocker.go`](../vmdocker/vmdocker.go) |
| Format/tag constants & validation | [`vmdocker/runtimemanager/schema/schema.go`](../vmdocker/runtimemanager/schema/schema.go), [`vmdocker/utils/utils.go`](../vmdocker/utils/utils.go) |
| public collect & match | [`vmdocker/capability/collect.go`](../vmdocker/capability/collect.go), [`publicmatch.go`](../vmdocker/capability/publicmatch.go) |
| Export (clone) | [`vmdocker/capability/export.go`](../vmdocker/capability/export.go) |
| Local e2e driver | [`cmd/vmme2e/main.go`](../cmd/vmme2e/main.go), [`scripts/e2e_capability.sh`](../scripts/e2e_capability.sh) |
