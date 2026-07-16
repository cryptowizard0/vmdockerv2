# Manual end-to-end round-trip test

[简体中文](manual-roundtrip-test_zh.md) · [Back to README](../README.md)

This guide verifies the complete VMDocker V2 lifecycle:

```text
build adapter → build Module → Spawn → change public state → Export → re-Spawn
```

Two routes are available:

- **Route A — full node:** the product path through a HyMatrix node and SDK. It requires Redis and local node initialization.
- **Route B — in-process capability path:** a faster host-side check without a node, Redis, Arweave, registration, or staking.

Use Route B for a quick implementation check. Use Route A to prove the real client-to-node round trip.

Commands assume `vmdockerv2` and `vmdocker_agent` are sibling directories:

```text
HymxWorkspace/
├── vmdockerv2/
└── vmdocker_agent/
```

## 1. Shared prerequisites

You need:

- Go 1.24.2+ for `vmdockerv2` and Go 1.25+ for `vmdocker_agent`.
- A running Docker daemon: `docker info` must succeed.
- Access to the selected base image.
- Redis for Route A only.

The examples below use `docker/sandbox-templates:claude-code`:

```bash
docker pull docker/sandbox-templates:claude-code
```

If private Go dependencies cannot be resolved, configure direct GitHub access before building the adapter:

```bash
export GOPRIVATE=github.com/hymatrix,github.com/xingj404-lab
```

### 1.1 Build `vmdocker-agent`

The adapter is injected into every Module image as `/usr/local/bin/vmdocker-agent`. Build it for the architecture used by the target image:

```bash
cd ../vmdocker_agent
scripts/build.sh
export VMDOCKER_AGENT_BIN="$PWD/build/vmdocker-agent"
cd ../vmdockerv2
```

Pass an architecture when cross-compiling:

```bash
../vmdocker_agent/scripts/build.sh amd64
# or: ../vmdocker_agent/scripts/build.sh arm64
```

The adapter architecture and image architecture must match.

### 1.2 Configure `.env`

Both `cmd/module` and `examples` read the root `.env`. Real environment variables take precedence.

```bash
cp .env.example .env
```

Set these values:

```dotenv
VMDOCKER_AGENT_BIN=/absolute/path/to/vmdocker_agent/build/vmdocker-agent
VMDOCKER_URL=http://127.0.0.1:8080
VMDOCKER_PRIVATE_KEY=0xreplace_with_a_local_development_key

VMDOCKER_MODULE_ID=
VMDOCKER_SCHEDULER=
VMDOCKER_EXPORT_PID=

RUNTIME_BACKEND=docker
RUNTIME_TYPE=claude
```

Use only a development key. `.env` is ignored by Git and must not be committed.

`RUNTIME_BACKEND=docker` is the quickest local path. If omitted, Linux defaults to `docker`, while macOS and Windows default to `sandbox`.

`RUNTIME_TYPE` is passed as `Container-Env-RUNTIME_TYPE` at Spawn time. It is not a `profile.toml` field.

## 2. Create the Module profile

Create this directory under `vmdockerv2`:

```text
myagent/
├── profile.toml
├── bin/
│   └── .keep
└── skills/
    └── soul.md
```

```bash
mkdir -p myagent/bin myagent/skills
printf 'keep\n' > myagent/bin/.keep
printf 'initial-state\n' > myagent/skills/soul.md
```

Create `myagent/profile.toml`:

```toml
[dockerfile]
FROM = "docker/sandbox-templates:claude-code"
bin = "bin"

[vmdocker]
public = ["~/skills/*"]
```

Important rules:

- `FROM` is a complete image reference and is used verbatim.
- `bin` is required. The directory may be empty, but it must exist.
- Put only your own executables in `bin/`. Do not copy `vmdocker-agent` there.
- `CMD` is optional. The adapter remains the image `ENTRYPOINT`.
- `[vmdocker].public` is a HOME-relative Export allowlist. Unlisted files remain private.

For Claude, `CMD` may be omitted because readiness checks that `claude` is available on `PATH`.

For an OpenClaw image, declare the gateway command when required by that image:

```toml
CMD = ["openclaw", "gateway", "--serve"]
```

## 3. Build the initial Module

Run from the `vmdockerv2` repository root:

```bash
go run ./cmd/module \
  -profile ./myagent/profile.toml \
  -agent-bin "$VMDOCKER_AGENT_BIN"
```

The command performs a real `docker build`, saves and compresses the image, packages `profile.toml` and `public.zip`, signs the bundle, and writes:

```text
mod-<MODULE_ID>.json
```

Record the printed Module ID, then place the file in the node's local Module store:

```bash
export MODULE_ID=<printed-module-id>
mkdir -p mod
cp "mod-${MODULE_ID}.json" "mod/mod-${MODULE_ID}.json"
```

Set `VMDOCKER_MODULE_ID` in `.env`, or export it in the shell:

```bash
export VMDOCKER_MODULE_ID="$MODULE_ID"
```

## 4. Route A — full node round trip

This route exercises the SDK, node, Redis, VM adapter, container, Export result, and second Spawn.

### A1. Start Redis

Use an existing Redis server, or start a disposable container:

```bash
docker run -d --name vmdockerv2-redis -p 6379:6379 redis:7-alpine
```

Verify it:

```bash
redis-cli -u redis://@localhost:6379/0 ping
```

### A2. Start the VMDocker node

Build and run the node from the repository root:

```bash
go build -o ./build/hymx-node ./cmd
./build/hymx-node --config ./cmd/config.yaml
```

Keep this terminal open. The node working directory determines where `mod/` and `sandbox_workspace/` are resolved.

Verify the HTTP service from another terminal:

```bash
curl -fsS http://127.0.0.1:8080/info
```

The repository config contains a public test key. Use it only for local development.

### A3. Initialize the local Token and Registry

On a fresh local node:

```bash
go run ./examples init
```

Set the scheduler to the node account returned by `/info`:

```bash
export VMDOCKER_SCHEDULER=<node-account-id>
```

Network-enabled configurations may require additional registration or staking. The default `joinNetwork: false` config is intended for local testing.

### A4. Spawn the initial Module

```bash
VMDOCKER_MODULE_ID="$MODULE_ID" \
VMDOCKER_SCHEDULER="$VMDOCKER_SCHEDULER" \
go run ./examples spawn
```

Record the printed process ID:

```text
spawned pid: <PID_1>
```

The seeded public file should exist under the node working directory:

```bash
export PID_1=<printed-process-id>
test -f "sandbox_workspace/${PID_1}/skills/soul.md"
cat "sandbox_workspace/${PID_1}/skills/soul.md"
```

### A5. Change public state

For this local manual test, update the public file directly in the process workspace:

```bash
printf 'evolved-state\n' > "sandbox_workspace/${PID_1}/skills/soul.md"
```

In a real workload, the running agent would make this change.

### A6. Preview and Export

Preview the files selected by `[vmdocker].public` without creating a Module:

```bash
VMDOCKER_EXPORT_DRY_RUN=1 go run ./examples export "$PID_1"
```

Create the exported Module:

```bash
go run ./examples export "$PID_1"
```

Success prints:

```text
exported module id: <EXPORTED_MODULE_ID>
the node wrote mod-<EXPORTED_MODULE_ID>.json into its module store
```

Export reuses the running process image. It does not rebuild the image and does not require `VMDOCKER_AGENT_BIN` on the node.

Only files allowed by `[vmdocker].public` are collected again. The image, `bin/`, installed tools, `RUN` results, and `CMD` remain unchanged.

The node writes the new Module to `mod/mod-<id>.json` and returns only its ID. The embedded image does not travel through Redis.

### A7. Re-Spawn the exported Module

No node restart is required. Module metadata is loaded from the local file when Spawn is handled.

```bash
export EXPORTED_MODULE_ID=<printed-exported-module-id>
VMDOCKER_MODULE_ID="$EXPORTED_MODULE_ID" \
VMDOCKER_SCHEDULER="$VMDOCKER_SCHEDULER" \
go run ./examples spawn
```

Record the second process ID and verify that the evolved public state was seeded:

```bash
export PID_2=<second-process-id>
cat "sandbox_workspace/${PID_2}/skills/soul.md"
```

Expected output:

```text
evolved-state
```

This proves the full build → Spawn → mutate → Export → re-Spawn round trip.

## 5. Route B — in-process capability round trip

This route exercises the production pack, seed, Export, and clone-seed code without a HyMatrix node or Redis. It does not test SDK or network routing.

### B1. Run the maintained capability test

```bash
bash scripts/e2e_capability.sh
```

The script always checks synthetic Module seeding. If Docker is available, it also verifies the workspace through a real bind-mounted container.

Enable the heavyweight real build and in-process Spawn check with:

```bash
RUN_REAL_SPAWN=1 bash scripts/e2e_capability.sh
```

The real Spawn test is `TestRealBuildSpawn`. There is no `TestBuildSpawnExportRespawn` test in the repository.

### B2. Manually verify Export and clone seeding

Build the thin driver that calls production capability code:

```bash
go build -o /tmp/vmme2e ./cmd/vmme2e
export RUN_DIR="$(mktemp -d)"
mkdir -p "$RUN_DIR/mod" "$RUN_DIR/author/skills"
```

Create a profile and initial public file:

```bash
cat > "$RUN_DIR/profile.toml" <<'TOML'
[dockerfile]
FROM = "alpine:3.20"
bin = "bin"

[vmdocker]
public = ["~/skills/*"]
TOML

printf 'initial-state\n' > "$RUN_DIR/author/skills/soul.md"
```

Pack and seed a synthetic source Module:

```bash
/tmp/vmme2e pack-synthetic \
  --profile "$RUN_DIR/profile.toml" \
  --public-dir "$RUN_DIR/author" \
  --out "$RUN_DIR/mod/mod-source.json"

(cd "$RUN_DIR" && /tmp/vmme2e seed-clone \
  --module-id source \
  --workspace "$RUN_DIR/ws1")
```

Change the public state and prepare a real reusable image archive:

```bash
printf 'evolved-state\n' > "$RUN_DIR/ws1/skills/soul.md"
docker pull alpine:3.20
docker save alpine:3.20 | gzip > "$RUN_DIR/image.tar.gz"
export IMAGE_ID="$(docker image inspect --format '{{.Id}}' alpine:3.20)"
```

Export the workspace and seed a second workspace from the exported Module:

```bash
/tmp/vmme2e export \
  --workspace "$RUN_DIR/ws1" \
  --image-archive "$RUN_DIR/image.tar.gz" \
  --image-name alpine:3.20 \
  --image-id "$IMAGE_ID" \
  --out "$RUN_DIR/mod/mod-exported.json"

(cd "$RUN_DIR" && /tmp/vmme2e seed-clone \
  --module-id exported \
  --workspace "$RUN_DIR/ws2")
```

Verify the round trip:

```bash
test "$(cat "$RUN_DIR/ws2/skills/soul.md")" = "evolved-state"
echo "round trip passed: $RUN_DIR"
```

## 6. Troubleshooting

### Module file not found

Module paths are relative to the node process working directory. Use `mod/mod-<id>.json`, and start the node from the same directory used in this guide.

### Spawn waits forever or fails readiness

Confirm that `/usr/local/bin/vmdocker-agent` is present and executable. Set `RUNTIME_TYPE=claude`, `openclaw`, or `test` to match the image behavior.

### Wrong runtime backend

Linux supports `docker` only. macOS and Windows default to `sandbox`; set `RUNTIME_BACKEND=docker` for the faster container path.

### Image architecture mismatch

Build `vmdocker-agent` for the same architecture as the base image. Use `scripts/build.sh amd64` or `scripts/build.sh arm64`.

### Export returns an ID, not Module bytes

`SendMessageAndWait` returns `Response{Id, Message}`. `examples/export.go` decodes `Message` as `VmmResult`; its `Data` field contains the exported Module ID.

The node persists the full Module because an image-backed Module can exceed Redis's result-size limits.

### Export omits a file

Only paths matched by `[vmdocker].public` are exported. Entries must start with `~/`; broad HOME-root globs and path escapes are rejected.

### Docker container exits immediately

The `docker` backend uses a read-only root filesystem. VMDocker bind-mounts a writable sibling directory at `/tmp`; inspect `sandbox_workspace/<pid>-tmp` for startup logs or temporary files.
