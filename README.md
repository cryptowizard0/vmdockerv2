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

### 🏷️ Module Format Requirements

VMDocker modules must follow specific format requirements to ensure proper container execution:

#### **ModuleFormat Specification**
- **Required Prefix**: `web.vmdocker-`
- **Format Pattern**: `web.vmdocker-{runtime}-{version}`
- **Examples**:
  - `web.vmdocker-golua-ao.v0.0.1`
  - `web.vmdocker-wasm-ao.v1.0.0`
  - `web.vmdocker-evm-ao.v2.1.0`

#### **Required Tags**

Every VMDocker module **MUST** include the following tags:

| Tag Name | Description | Example |
|----------|-------------|----------|
| `Image-Name` | Docker image name and tag | `chriswebber/docker-golua:v0.0.2` |
| `Image-ID` | Docker image SHA256 digest | `sha256:b2e104cdcb5c09a8f213aefcadd451cbabfda1f16c91107e84eef051f807d45b` |
| `Image-Source` | Module image source selector | `module-data` |
| `Image-Archive-Format` | Embedded image archive format | `docker-save+gzip` |

> ⚠️ **Important**: `Image-Name`, `Image-ID`, `Image-Source=module-data`, and `Image-Archive-Format=docker-save+gzip` are mandatory. Legacy `Build-*` modules are no longer supported.

#### **What A Module Contains**

VMDocker sandbox modules no longer store a Dockerfile or build recipe for spawn-time builds.

The generated module now contains:

- image/runtime metadata tags such as `Start-Command`, `Sandbox-Agent`, `Openclaw-Version`
- final image metadata in tags: `Image-Name`, `Image-ID`
- the actual Docker image archive inside bundle `data`

`Runtime-Backend` is no longer stored in the module. Backend selection now happens at spawn time.

The image archive format is:

```text
docker save <image> | gzip
```

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

Claude-specific developer guide:

- [Claude Runtime In VMDocker](/Users/webbergao/work/src/HymxWorkspace/vmdocker/docs/claude-runtime.md)

Follow these steps to create, validate, and run a sandbox module end to end.

**Step 1: Prepare The Final Image**

Choose one of these two generation modes in `vmdocker_agent/.env`:

- Pull mode:
  - set `VMDOCKER_SANDBOX_IMAGE_NAME`
  - optionally set `VMDOCKER_SANDBOX_IMAGE_ID`
- Build mode:
  - set `VMDOCKER_BUILD_DOCKERFILE`
  - set `VMDOCKER_BUILD_CONTEXT_DIR`
  - set `VMDOCKER_BUILD_TAG`

Common required entries:

```dotenv
VMDOCKER_URL=http://127.0.0.1:8080
VMDOCKER_PRIVATE_KEY=
```

**Step 2: Generate The Module**

Run the generator from `vmdocker_agent`:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
go run ./cmd/module
```

This command:

- prepares the final local image
- exports it with `docker save | gzip`
- writes a local bundle file `mod-<module-id>.json`
- prints the generated module id

Example output:

```bash
[module] generate and save module success, id <generated-module-id>
[module] local bundle file: mod-<generated-module-id>.json
```

**Step 3: Make The Module File Available To The Node**

For local testing, copy the generated file into the VMDocker node working directory:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker
mkdir -p mod
cp ../vmdocker_agent/mod/mod-<generated-module-id>.json ./mod/mod-<generated-module-id>.json
```

If the node downloads the module from the network instead, Hymx will cache the same bundle as `mod/mod-<module-id>.json` automatically after the first download.

**Step 4: Start The VMDocker Node**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker
go build -o ./build/hymx-node ./cmd
./build/hymx-node --config ./config.yaml
```

**Step 5: Configure Example Environment**

In `vmdocker/examples/.env`, point both ids to the generated module:

```dotenv
VMDOCKER_MODULE_ID=<generated-module-id>
OPENCLAW_MODULE_ID=<generated-module-id>
OPENCLAW_PROVIDER=zen
OPENCLAW_MODEL=plan
# Optional: if you omit OPENCLAW_PROVIDER, a fully-qualified model like kimi-coding/k2p5 still works.
```

**Step 6: Spawn The Runtime**

General spawn:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker
go run ./examples spawn
```

OpenClaw spawn:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker
go run ./examples openclaw_spawn
```

The example forwards `provider`, `model`, and `apiKey` as spawn tags to `vmdocker_agent`. If `OPENCLAW_PROVIDER` is set, provider selection is explicit and the runtime will normalize the final model to `<provider>/<model-suffix>`.

**Step 7: Configure Telegram Without Pairing**

OpenClaw follows the official Telegram rules:

- `dmPolicy=open` is valid
- but `allowFrom` must include `"*"` for open DM access

Recommended example settings:

```dotenv
OPENCLAW_TELEGRAM_DM_POLICY=open
OPENCLAW_TELEGRAM_ALLOW_FROM=*
```

Then run:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker
go run ./examples openclaw_tg
```

The runtime will patch `openclaw.json`, restart the gateway if needed, and enable Telegram with open DMs.

**Step 8: Validate Cold Start From Module Data**

To verify that VMDocker can restore the image from the module file instead of local Docker cache:

1. Delete the local image matching `Image-Name`
2. Spawn again with the same module id
3. Confirm the runtime still starts successfully

This validates the full recovery path:

```text
module file -> bundle data -> gunzip -> docker image load -> sandbox start
```

#### **Validation Process**

VMDocker automatically validates modules using the `checkModule` function:

1. ✅ **ModuleFormat Check**: verifies the module format
2. ✅ **Image-Name Check**: ensures `Image-Name` exists
3. ✅ **Image-ID Check**: ensures `Image-ID` exists
4. ✅ **Image-Source Check**: requires `Image-Source=module-data`
5. ✅ **Image-Archive-Format Check**: requires `Image-Archive-Format=docker-save+gzip`

If any validation fails, the module will be rejected and container creation will fail.

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
