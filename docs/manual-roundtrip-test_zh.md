# 手动端到端往返测试

[English](manual-roundtrip-test.md) · [返回 README](../README_zh.md)

本文验证 VMDocker V2 的完整生命周期：

```text
构建 Adapter → 构建 Module → Spawn → 修改公开状态 → Export → 再次 Spawn
```

提供两条路线：

- **路线 A——完整节点：** 通过 HyMatrix 节点和 SDK 走真实产品路径，需要 Redis 和本地节点初始化。
- **路线 B——进程内能力路径：** 不需要节点、Redis、Arweave、注册或质押，适合快速验证宿主侧能力。

建议先用路线 B 验证实现，再用路线 A 验证真实的客户端到节点往返流程。

以下命令假设 `vmdockerv2` 和 `vmdocker_agent` 是同级目录：

```text
HymxWorkspace/
├── vmdockerv2/
└── vmdocker_agent/
```

## 1. 共同前置条件

需要准备：

- `vmdockerv2` 使用 Go 1.24.2+，`vmdocker_agent` 使用 Go 1.25+。
- Docker daemon 正常运行，`docker info` 可以成功。
- 可以拉取所选基础镜像。
- 路线 A 还需要 Redis。

下面使用 `docker/sandbox-templates:claude-code`：

```bash
docker pull docker/sandbox-templates:claude-code
```

如果私有 Go 依赖无法解析，在构建 Adapter 前配置 GitHub 直连：

```bash
export GOPRIVATE=github.com/hymatrix,github.com/xingj404-lab
```

### 1.1 构建 `vmdocker-agent`

Adapter 会作为 `/usr/local/bin/vmdocker-agent` 注入每个 Module 镜像。它的架构必须与目标镜像一致：

```bash
cd ../vmdocker_agent
scripts/build.sh
export VMDOCKER_AGENT_BIN="$PWD/build/vmdocker-agent"
cd ../vmdockerv2
```

交叉编译时指定架构：

```bash
../vmdocker_agent/scripts/build.sh amd64
# 或：../vmdocker_agent/scripts/build.sh arm64
```

Adapter 和镜像架构必须匹配。

### 1.2 配置 `.env`

`cmd/module` 和 `examples` 都读取仓库根目录的 `.env`。真实环境变量优先。

```bash
cp .env.example .env
```

设置以下内容：

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

只能使用开发密钥。`.env` 已被 Git 忽略，不要提交该文件。

`RUNTIME_BACKEND=docker` 是最快的本地路径。不设置时，Linux 默认使用 `docker`，macOS 和 Windows 默认使用 `sandbox`。

`RUNTIME_TYPE` 会在 Spawn 时通过 `Container-Env-RUNTIME_TYPE` 传入，它不是 `profile.toml` 字段。

## 2. 创建 Module Profile

在 `vmdockerv2` 下创建：

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

创建 `myagent/profile.toml`：

```toml
[dockerfile]
FROM = "docker/sandbox-templates:claude-code"
bin = "bin"

[vmdocker]
public = ["~/skills/*"]
```

重要规则：

- `FROM` 是完整镜像引用，会被原样使用。
- `bin` 必填。目录可以为空，但必须存在。
- `bin/` 只放自己的可执行程序，不要把 `vmdocker-agent` 复制进去。
- `CMD` 可选，Adapter 始终是镜像的 `ENTRYPOINT`。
- `[vmdocker].public` 是 HOME 相对的 Export 白名单，未列出的文件保持私有。

Claude 的就绪条件是 `claude` 存在于 `PATH`，因此可以不设置 `CMD`。

OpenClaw 镜像需要启动网关时，可以声明：

```toml
CMD = ["openclaw", "gateway", "--serve"]
```

## 3. 构建初始 Module

在 `vmdockerv2` 仓库根目录执行：

```bash
go run ./cmd/module \
  -profile ./myagent/profile.toml \
  -agent-bin "$VMDOCKER_AGENT_BIN"
```

该命令会执行真实的 `docker build`，保存并压缩镜像，打包 `profile.toml` 和 `public.zip`，签名后生成：

```text
mod-<MODULE_ID>.json
```

记录输出的 Module ID，并将文件放入节点本地 Module 存储：

```bash
export MODULE_ID=<printed-module-id>
mkdir -p mod
cp "mod-${MODULE_ID}.json" "mod/mod-${MODULE_ID}.json"
```

在 `.env` 中设置 `VMDOCKER_MODULE_ID`，或直接导出环境变量：

```bash
export VMDOCKER_MODULE_ID="$MODULE_ID"
```

## 4. 路线 A——完整节点往返

该路线会经过 SDK、节点、Redis、VM Adapter、容器、Export 结果和第二次 Spawn。

### A1. 启动 Redis

使用已有 Redis，或启动临时容器：

```bash
docker run -d --name vmdockerv2-redis -p 6379:6379 redis:7-alpine
```

验证 Redis：

```bash
redis-cli -u redis://@localhost:6379/0 ping
```

### A2. 启动 VMDocker 节点

从仓库根目录构建并启动节点：

```bash
go build -o ./build/hymx-node ./cmd
./build/hymx-node --config ./cmd/config.yaml
```

保持该终端运行。节点工作目录决定 `mod/` 和 `sandbox_workspace/` 的解析位置。

在另一个终端验证 HTTP 服务：

```bash
curl -fsS http://127.0.0.1:8080/info
```

仓库配置包含公开的测试密钥，只能用于本地开发。

### A3. 初始化本地 Token 和 Registry

首次运行本地节点时执行：

```bash
go run ./examples init
```

将 `/info` 返回的节点账户设置为 scheduler：

```bash
export VMDOCKER_SCHEDULER=<node-account-id>
```

加入真实网络的配置可能还需要注册或质押。默认的 `joinNetwork: false` 配置用于本地测试。

### A4. Spawn 初始 Module

```bash
VMDOCKER_MODULE_ID="$MODULE_ID" \
VMDOCKER_SCHEDULER="$VMDOCKER_SCHEDULER" \
go run ./examples spawn
```

记录输出的进程 ID：

```text
spawned pid: <PID_1>
```

公开文件应已种入节点工作目录：

```bash
export PID_1=<printed-process-id>
test -f "sandbox_workspace/${PID_1}/skills/soul.md"
cat "sandbox_workspace/${PID_1}/skills/soul.md"
```

### A5. 修改公开状态

本地手动测试可以直接修改进程工作区中的公开文件：

```bash
printf 'evolved-state\n' > "sandbox_workspace/${PID_1}/skills/soul.md"
```

真实工作负载中，该变化由运行中的 Agent 产生。

### A6. 预览并 Export

先预览 `[vmdocker].public` 会选择哪些文件，不生成 Module：

```bash
VMDOCKER_EXPORT_DRY_RUN=1 go run ./examples export "$PID_1"
```

生成导出的 Module：

```bash
go run ./examples export "$PID_1"
```

成功时输出：

```text
exported module id: <EXPORTED_MODULE_ID>
the node wrote mod-<EXPORTED_MODULE_ID>.json into its module store
```

Export 会复用运行中进程的镜像，不会重新构建，也不要求节点配置 `VMDOCKER_AGENT_BIN`。

只有 `[vmdocker].public` 允许的文件会被重新收集。镜像、`bin/`、已安装工具、`RUN` 结果和 `CMD` 保持不变。

节点将新 Module 写入 `mod/mod-<id>.json`，结果只返回 ID，内嵌镜像不会经过 Redis。

### A7. 再次 Spawn 导出的 Module

不需要重启节点。处理 Spawn 时，节点会从本地文件加载 Module 元数据。

```bash
export EXPORTED_MODULE_ID=<printed-exported-module-id>
VMDOCKER_MODULE_ID="$EXPORTED_MODULE_ID" \
VMDOCKER_SCHEDULER="$VMDOCKER_SCHEDULER" \
go run ./examples spawn
```

记录第二个进程 ID，并验证修改后的公开状态已被种入：

```bash
export PID_2=<second-process-id>
cat "sandbox_workspace/${PID_2}/skills/soul.md"
```

预期输出：

```text
evolved-state
```

至此验证了完整的构建 → Spawn → 修改 → Export → 再次 Spawn 往返流程。

## 5. 路线 B——进程内能力往返

该路线调用生产环境的打包、种入、Export 和克隆种入代码，不需要 HyMatrix 节点或 Redis，但不会验证 SDK 和网络路由。

### B1. 运行维护中的能力测试

```bash
bash scripts/e2e_capability.sh
```

脚本始终验证合成 Module 的工作区种入。Docker 可用时，还会通过真实 bind mount 容器检查工作区。

启用真实构建和进程内 Spawn 的重型检查：

```bash
RUN_REAL_SPAWN=1 bash scripts/e2e_capability.sh
```

真实 Spawn 测试名是 `TestRealBuildSpawn`。仓库中不存在 `TestBuildSpawnExportRespawn`。

### B2. 手动验证 Export 和克隆种入

构建调用生产能力代码的轻量驱动：

```bash
go build -o /tmp/vmme2e ./cmd/vmme2e
export RUN_DIR="$(mktemp -d)"
mkdir -p "$RUN_DIR/mod" "$RUN_DIR/author/skills"
```

创建 Profile 和初始公开文件：

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

打包并种入一个合成源 Module：

```bash
/tmp/vmme2e pack-synthetic \
  --profile "$RUN_DIR/profile.toml" \
  --public-dir "$RUN_DIR/author" \
  --out "$RUN_DIR/mod/mod-source.json"

(cd "$RUN_DIR" && /tmp/vmme2e seed-clone \
  --module-id source \
  --workspace "$RUN_DIR/ws1")
```

修改公开状态，并准备可复用的真实镜像归档：

```bash
printf 'evolved-state\n' > "$RUN_DIR/ws1/skills/soul.md"
docker pull alpine:3.20
docker save alpine:3.20 | gzip > "$RUN_DIR/image.tar.gz"
export IMAGE_ID="$(docker image inspect --format '{{.Id}}' alpine:3.20)"
```

Export 工作区，再从导出的 Module 种入第二个工作区：

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

验证往返结果：

```bash
test "$(cat "$RUN_DIR/ws2/skills/soul.md")" = "evolved-state"
echo "round trip passed: $RUN_DIR"
```

## 6. 常见问题

### 找不到 Module 文件

Module 路径相对于节点进程的工作目录。使用 `mod/mod-<id>.json`，并从本文使用的同一目录启动节点。

### Spawn 一直等待或就绪失败

确认 `/usr/local/bin/vmdocker-agent` 存在且可执行。根据镜像行为设置 `RUNTIME_TYPE=claude`、`openclaw` 或 `test`。

### 运行后端错误

Linux 只支持 `docker`。macOS 和 Windows 默认使用 `sandbox`；设置 `RUNTIME_BACKEND=docker` 可使用更快的容器路径。

### 镜像架构不匹配

`vmdocker-agent` 必须与基础镜像架构一致。使用 `scripts/build.sh amd64` 或 `scripts/build.sh arm64`。

### Export 返回 ID，而不是 Module 字节

`SendMessageAndWait` 返回 `Response{Id, Message}`。`examples/export.go` 将 `Message` 解码为 `VmmResult`，其中 `Data` 字段保存导出的 Module ID。

节点会直接持久化完整 Module，因为内嵌镜像的 Module 可能超过 Redis 的结果大小限制。

### Export 缺少文件

只有 `[vmdocker].public` 匹配的路径会被导出。条目必须以 `~/` 开头；过宽的 HOME 根目录 glob 和路径逃逸会被拒绝。

### Docker 容器立即退出

`docker` 后端使用只读根文件系统。VMDocker 会把一个可写的同级目录挂载到 `/tmp`；可检查 `sandbox_workspace/<pid>-tmp` 中的启动日志或临时文件。
