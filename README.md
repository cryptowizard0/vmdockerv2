<div align="center">

# 🐳 VMDocker V2

**A Docker-based Virtual Machine Implementation for HyMatrix Computing Network**

[![Go Version](https://img.shields.io/badge/Go-1.24.2-blue.svg)](https://golang.org/)
[![Docker](https://img.shields.io/badge/Docker-28.0.x-blue.svg)](https://www.docker.com/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![HyMatrix](https://img.shields.io/badge/HyMatrix-Compatible-orange.svg)](https://hymatrix.com/)

</div>

## 📖 Overview

**VMDocker** is a high-performance, Docker-based virtual machine implementation designed for the HyMatrix computing network. It serves as a universal virtual machine extension that can be seamlessly mounted to HyMatrix nodes, enabling scalable and verifiable computation execution.

### 🌟 Key Features

- **🔌 Universal VM Interface**: Compatible with standard HyMatrix VM protocol
- **🐳 Docker-based**: Leverages Docker containers for isolated computation environments
- **🔄 Multi-Architecture Support**: Supports EVM, WASM, AO, LLM model services, and more
- **📊 Checkpoint & Restore**: Advanced state management with CRIU integration
- **⚡ High Performance**: Optimized for scalable computation workloads
- **🔗 AO Compatible**: Full support for AO protocol containers

### 🏗️ Architecture

```
┌─────────┐    ┌──────────┐    ┌───────────┐
│ HyMatrix│───▶│VMDocker  │───▶│Container  │
│  Node   │    │ Manager  │    │(EVM/WASM) │
└─────────┘    └──────────┘    └───────────┘
```

### 🔗 About HyMatrix

**HyMatrix** is an infinitely scalable decentralized computing network that decouples computation from consensus by anchoring execution logs in immutable storage (Arweave), enabling verifiable, trustless computation anywhere.

🌐 **Learn more**: [https://hymatrix.com/](https://hymatrix.com/)

### 🛠️ VM Interface

VMDocker implements the standard HyMatrix VM interface:

```go
// hymx/vmm/schema/schema.go
type Vm interface {
    Apply(from string, meta Meta) (res *Result, err error)
    Checkpoint() (data string, err error)
    Restore(data string) error
    Close() error
}
```

**Supported Container Types**:
- 🔷 **EVM**: Ethereum Virtual Machine
- 🟦 **WASM**: WebAssembly runtime
- 🟠 **AO**: Arweave AO protocol ([Container Repository](https://github.com/cryptowizard0/vmdocker_container))
- 🤖 **LLM**: Large Language Model services
- ➕ **Custom**: Any containerized computation environment

## 🚀 Getting Started

### 📋 Prerequisites

| Component | Version | Platform | Required |
|-----------|---------|----------|----------|
| **Operating System** | Linux | Any | ✅ |
| **Go** | 1.24.2 | Any | ✅ |
| **Docker** | 28.0.x | Any | ✅ |
| **Redis** | Latest | Any | ✅ |
| **Clang/GCC** | Latest | Any | ✅ (for CGO) |
| **CRIU** | v4.1 | Linux only | ⚠️ (for checkpoint) |

> ⚠️ **Note**: CRIU is only required for checkpoint functionality and is Linux-specific. macOS users can skip CRIU installation.

### 📦 Installation

#### 1. Clone Repository

```bash
git clone https://github.com/cryptowizard0/vmdockerv2.git
cd vmdockerv2
```

#### 2. Install Dependencies

```bash
go mod tidy
```

#### 3. Build VMDocker

```bash
go build -o ./build/hymx-node ./cmd
```

#### 4. Install System Dependencies

**Ubuntu/Debian:**
```bash
sudo apt-get update
sudo apt-get install gcc build-essential redis-server
```

**CentOS/RHEL:**
```bash
sudo yum install gcc gcc-c++ make redis
```

### 🔧 Optional: CRIU Installation (Linux Only)

> 📝 **Required for**: Checkpoint and restore functionality
> 🖥️ **Platform**: Linux systems only

#### Install CRIU v4.1

```bash
# Download CRIU v4.1 source code
wget https://github.com/checkpoint-restore/criu/archive/criu_v4.1.tar.gz
tar -xzf criu_v4.1.tar.gz
cd criu-criu_v4.1

# Compile and install
make
sudo make install

# Verify installation
criu check
# Expected output: "Looks good."
```

### 🐳 Docker Configuration

> ⚠️ **Important**: Docker version `28.0.x` is required for optimal compatibility.

#### Enable Experimental Features

Docker checkpoint requires experimental features to be enabled:

```bash
# Create Docker daemon configuration
sudo mkdir -p /etc/docker

# Enable experimental features
sudo tee /etc/docker/daemon.json <<-'EOF'
{
  "experimental": true
}
EOF

# Restart Docker service
sudo systemctl restart docker

# Verify experimental features are enabled
docker info | grep "Experimental"
# Expected output: "Experimental: true"
```

## ⚙️ Configuration

### 📝 Create Configuration File

VMDocker uses standard HyMatrix configuration format. Create a `config.yaml` file:

```yaml
# 🌐 Node Service Configuration
port: :8080
ginMode: release  # Options: "debug", "release"

# 🔴 Redis Configuration
redisURL: redis://@localhost:6379/0

# 🌍 Storage & Network
arweaveURL: https://arweave.net
hymxURL: http://127.0.0.1:8080

# 🔐 Node Identity (Wallet)
prvKey: 0x64dd2342616f385f3e8157cf7246cf394217e13e8f91b7d208e9f8b60e25ed1b
keyfilePath:  # Optional: path to keyfile instead of prvKey

# ℹ️ Node Information
nodeName: test1
nodeDesc: first test node
nodeURL: http://127.0.0.1:8080

# 🔗 Network Participation
joinNetwork: false  # Set to true for production network
```

### 📊 Configuration Reference

| Field | Type | Description | Example |
|-------|------|-------------|----------|
| `port` | string | HTTP server port | `:8080` |
| `ginMode` | string | Gin framework mode | `release` or `debug` |
| `redisURL` | string | Redis connection URL | `redis://@localhost:6379/0` |
| `arweaveURL` | string | Arweave gateway URL | `https://arweave.net` |
| `hymxURL` | string | Local node URL for SDK calls | `http://127.0.0.1:8080` |
| `prvKey` | string | Ethereum private key (hex) | `0x64dd...` |
| `keyfilePath` | string | Alternative to prvKey | `./keyfile.json` |
| `nodeName` | string | Node identifier | `my-node` |
| `nodeDesc` | string | Node description | `Production node` |
| `nodeURL` | string | Public node URL | `https://my-node.com` |
| `joinNetwork` | boolean | Join HyMatrix network | `false` (testing), `true` (production) |

> 📚 **For detailed configuration options**, see [HyMatrix Configuration Documentation](https://docs.hymatrix.com/docs/join-the-network/setup)

## 📋 Module Configuration

> 📘 **Full profile-driven build & spawn guide:** [docs/profile-module-guide.md](docs/profile-module-guide.md).
> V2 modules are built from a declarative `profile.toml`; that guide is the authoritative, step-by-step reference. This section is a summary.

### 🏷️ Module Format Requirements

VMDocker modules must follow specific format requirements to ensure proper container execution:

#### **ModuleFormat Specification**

V2 uses a single module format constant (no per-runtime prefix):

- **Module Format**: `hymx.vmdockerv2.v0.0.1`

The node mounts the spawn handler under this format (`s.Mount(ModuleFormat, vmdocker.Spawn)`); any other format is rejected with `unsupported module format`.

#### **Required Tags**

Every VMDocker module **MUST** include the following tags:

| Tag Name | Description | Example |
|----------|-------------|----------|
| `Image-Name` | Docker image name and tag | `chriswebber/docker-golua:v0.0.2` |
| `Image-ID` | Docker image SHA256 digest | `sha256:b2e104cdcb5c09a8f213aefcadd451cbabfda1f16c91107e84eef051f807d45b` |
| `Image-Source` | Module image source selector | `module-data` |
| `Image-Archive-Format` | Embedded image archive format | `container-tar+image.tar.gz` |

> ⚠️ **Important**: `Image-Name`, `Image-ID`, `Image-Source=module-data`, and `Image-Archive-Format` are mandatory. The current builder emits `Image-Archive-Format=container-tar+image.tar.gz`; the loader still accepts the legacy `docker-save+gzip`. `Build-Type` / legacy `Build-*` modules are rejected.

#### **What A Module Contains**

VMDocker sandbox modules no longer store a Dockerfile or build recipe for spawn-time builds.

The generated module bundle `data` is a gzipped container-tar carrying:

- `image.tar.gz` — the image, produced by `docker save <image> | gzip`
- `profile.toml` — the declarative build recipe, also seeded into the workspace on spawn
- `public.zip` — optional; the exportable files selected by `[vmdocker].public`

plus metadata tags: `Image-Name`, `Image-ID`, `Image-Source=module-data`, `Image-Archive-Format=container-tar+image.tar.gz`, `Capability-Public`, `Member-Image-SHA256`.

`Runtime-Backend` is not stored in the module. Backend selection happens at spawn time.

At spawn time, VMDocker behaves like this:

1. Check whether local Docker already has `Image-Name` with the expected `Image-ID`
2. If it exists, start immediately
3. If it does not exist, read `mod/mod-<module-id>.json`
4. Decode bundle `data`, gunzip it, run `docker image load`
5. Re-tag and verify the restored image
6. Start the sandbox/runtime

#### **Runtime Tags And Spawn Tags**

Backend and startup behavior are split on purpose:

- Module tags describe the image itself
- Spawn tags describe how this specific run should execute

Recommended module tags:

| Tag Name | Where | Description | Example |
|----------|-------|-------------|----------|
| `Start-Command` | module | Default runtime entry command for both docker and sandbox backends | `/usr/local/bin/start-vmdocker-agent.sh` |
| `Sandbox-Agent` | module | Docker Sandbox agent type | `shell` |
| `Openclaw-Version` | module | Optional runtime metadata | `2026.3.13` |

Supported spawn-time runtime tags:

| Tag Name | Where | Description | Example |
|----------|-------|-------------|----------|
| `Runtime-Backend` | spawn | Runtime backend selector | `docker`, `sandbox` |
| `Start-Command` | spawn | Optional one-off override for module `Start-Command` | `/app/custom-entrypoint --serve` |

Backend rules:

- If spawn sets `Runtime-Backend`, VMDocker uses that backend
- If spawn omits it, VMDocker chooses by OS
- macOS / Windows default to `sandbox`
- Linux defaults to `docker`
- Linux rejects `Runtime-Backend=sandbox`

`Start-Command` rules:

- `Start-Command` should normally live in the module
- Spawn may override it for testing or one-off runtime changes
- The value is parsed as `command + args`, not as a shell fragment

#### **Runtime Workspace And Environment**

Both `docker` and `sandbox` now follow the same fixed runtime workspace contract.

Given the default workspace root, VMDocker resolves the per-instance workspace as:

```text
<workspace-root>/sandbox_workspace/<pid>
```

The runtime then uses these paths inside that workspace:

| Environment Variable | Default Value |
|----------------------|---------------|
| `OPENCLAW_HOME` | `<workspace>` |
| `OPENCLAW_STATE_DIR` | `<workspace>/.openclaw` |
| `OPENCLAW_CONFIG_PATH` | `<workspace>/.openclaw/openclaw.json` |
| `OPENCLAW_AGENT_WORKSPACE` | `<workspace>/.openclaw/workspace` |
| `HOME` | `<workspace>/.home` |
| `TMPDIR` | `<workspace>/.tmp` |
| `XDG_CONFIG_HOME` | `<workspace>/.xdg/config` |
| `XDG_CACHE_HOME` | `<workspace>/.xdg/cache` |
| `XDG_STATE_HOME` | `<workspace>/.xdg/state` |

If these env vars are already provided explicitly, VMDocker preserves the explicit value.

#### **Current Runtime Confinement**

The current runtime policy is:

- `docker`: container root filesystem is read-only; the mapped instance workspace remains writable
- `sandbox`: runtime startup hardens common writable locations such as `/tmp`, `/var/tmp`, `/home/agent`, and `/workspace`, while keeping the mapped instance workspace writable

This means both backends are intended to write runtime state only inside the mapped per-instance workspace.

#### **End-To-End Workflow**

The full, current workflow — writing `profile.toml`, building the module, and spawning it — lives in
**[docs/profile-module-guide.md](docs/profile-module-guide.md)**. Claude-specific runtime notes:
[docs/claude-runtime.md](docs/claude-runtime.md). In short:

```bash
# 1) Build the linux platform adapter from the sibling repo vmdocker_agent
cd ../vmdocker_agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/vmdocker-agent .

# 2) Build + sign a module from your profile.toml (needs VMDOCKER_PRIVATE_KEY)
cd ../vmdockerv2
export VMDOCKER_URL=http://127.0.0.1:8080
export VMDOCKER_PRIVATE_KEY=0x<your-key>
go run ./cmd/module -profile ./mymod/profile.toml -agent-bin /tmp/vmdocker-agent
#  -> [module] saved module <id> -> mod-<id>.json

# 3) Make the module available to the node, then start it
mkdir -p ./mod && cp mod-<id>.json ./mod/mod-<id>.json
go build -o ./build/hymx-node ./cmd
./build/hymx-node --config ./config.yaml
```

**Validate cold start from module data:** delete the local image matching `Image-Name` and spawn again
with the same module id; the runtime restores it via
`module file -> bundle data -> gunzip -> docker image load -> start`.

**Local end-to-end checks (no chain):** run `bash scripts/e2e_capability.sh` (add `RUN_REAL_SPAWN=1`
for a real build + spawn). See the guide for `cmd/vmme2e` (`seed` / `seed-clone` / `export` /
`pack-synthetic`) details.

#### **Validation Process**

VMDocker validates a module when it resolves the runtime spec (`CheckModuleFormat` / `imageInfoFromTags` in `vmdocker/utils/utils.go`):

1. ✅ **Module Format**: must be `hymx.vmdockerv2.v0.0.1`
2. ✅ **Image-Name**: must be present
3. ✅ **Image-ID**: must be present
4. ✅ **Image-Source**: must equal `module-data`
5. ✅ **Image-Archive-Format**: must be `container-tar+image.tar.gz` or `docker-save+gzip`
6. 🚫 **Build-Type**: rejected — legacy build modules are no longer supported

If any check fails, the module is rejected and container creation fails.

## 🚀 Running VMDocker

### 1. 🔴 Start Redis Server

Ensure Redis is running before starting VMDocker:

```bash
# Ubuntu/Debian
sudo systemctl start redis-server
sudo systemctl enable redis-server

# CentOS/RHEL
sudo systemctl start redis
sudo systemctl enable redis

# macOS (with Homebrew)
brew services start redis
```

### 2. 🚀 Launch VMDocker Node

```bash
# From the project root directory
./build/hymx-node --config ./config.yaml
```

### 3. ✅ Verify Startup

Successful startup will display:

```
INFO[07-25|00:00:01] server is running   module=node-v0.0.1 wallet=0x... port=:8080
```

## 🌐 Network Participation

### 🔗 Join HyMatrix Network

To participate as a network node operator:

1. **Configure for Production**
   ```yaml
   joinNetwork: true
   nodeURL: https://your-public-domain.com  # Your public URL
   ```

2. **Stake HMX Tokens**
   - Acquire the required HMX tokens
   - Complete the staking process

3. **Complete Registration**
   - Submit node registration
   - Wait for network acceptance

### 💰 Rewards

Participating nodes earn rewards for:
- ⚡ **Computation execution**
- 📝 **Log submission**
- 🔗 **Network services**
- 🛡️ **Network security**

> 📖 **For detailed network joining instructions**, see [HyMatrix Network Documentation](https://docs.hymatrix.com/docs/category/join-the-network)

## Using

### Run AOS Client

vmdocker is an AO-compatible system. Use the modified AOS to connect to vmdocker.

1. Clone AOS repository:
   ```bash
   git clone https://github.com/cryptowizard0/aos
   ```

2. Install Node.js dependencies:
   ```bash
   npm install
   ```

3. Start AOS client:
    - `cu-url` and `mu-url` should be the same as the vmdocker node url
    - `scheduler` is the vmdocker node id
   ```bash
   DEBUG=true node src/index.js \
    --cu-url=http://127.0.0.1:8080 \
    --mu-url=http://127.0.0.1:8080 \
    --scheduler=0x972AeD684D6f817e1b58AF70933dF1b4a75bfA51 \
    test_name
   ``` 

   After the first launch, please record your Process ID. To reconnect to the specific process later, use the following command:

   ```bash
   DEBUG=true node src/index.js \
    --cu-url=http://127.0.0.1:8080 \
    --mu-url=http://127.0.0.1:8080 \
    --scheduler=0x972AeD684D6f817e1b58AF70933dF1b4a75bfA51 \
    {{processId}}
   ```

### Examples

Reference implementations are available in the `examples` directory.
