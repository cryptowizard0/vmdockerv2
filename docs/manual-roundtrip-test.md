# 手动端到端往返测试

打包 adapter → 用 `profile.toml` 构建 module → spawn → 把运行中的进程 export 成新 module → 再
spawn。两条路线:

- **路线 A —— 完整节点。** 真实产品路径:跑起 hymx 节点,通过 SDK 驱动。需要真基础设施(Redis、
  Arweave 网关)以及节点引导(init / registry / 质押)。较重。
- **路线 B —— 进程内(建议先跑)。** 用同一套 build → spawn → export → 再 spawn 的能力代码,直接
  通过 `vmdocker.Spawn` + `vm.Apply` 驱动,**不需要节点、Redis、Arweave、质押**。几分钟出结果。

下文路径假设工作区在 `/Users/webbergao/work/src/HymxWorkspace`。

---

## 前置(两条路线通用)

- Docker 在跑。
- 能拉到 claude 基础镜像:`docker pull docker/sandbox-templates:claude-code`。
- 有 GitHub SSH 访问(adapter 的 `claude-gw` / `hymatrix` 依赖走 github 直连):
  `export GOPRIVATE=github.com/hymatrix,github.com/xingj404-lab`。

### 第 0 步 —— 打包 adapter(vmdocker_agent)

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git switch feature/adapter-entrypoint-only
scripts/build.sh                       # -> build/vmdocker-agent (linux, 宿主 arch)
export VMDOCKER_AGENT_BIN="$PWD/build/vmdocker-agent"
```

`scripts/build.sh <goarch>` 可交叉编译到指定 arch;要与基础镜像架构一致(Apple Silicon 拉的是
arm64 的 claude-code 镜像,所以宿主默认 arch 正好对)。

### 用 `.env` 一次配好(`cmd/module` 和 `examples` 都读)

`cmd/module` 和 `examples/` 都会自动读取 vmdockerv2 **根目录的 `.env`**(真实环境变量优先;
`VMDOCKER_ENV_FILE` 可指定别的路径)。所以这些 `VMDOCKER_*` 填一次就好,**下面各步命令里的
`export VMDOCKER_*` 行都可以省掉。**

`.env` 已 gitignore(含私钥,别提交);仓库里有个不含密钥的模板 `.env.example`:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
cp .env.example .env      # 然后按下面填
```

```dotenv
# 构建 / Export 时把哪个 adapter 二进制烤进镜像(第 0 步编出来的路径)
VMDOCKER_AGENT_BIN=/Users/webbergao/work/src/HymxWorkspace/vmdocker_agent/build/vmdocker-agent
# 节点 URL:cmd/module 上传 module、examples 发消息都打到这
VMDOCKER_URL=http://127.0.0.1:8080
# 给 module 签名的私钥(cmd/config.yaml 里有个测试 key)
VMDOCKER_PRIVATE_KEY=0x64dd2342616f385f3e8157cf7246cf394217e13e8f91b7d208e9f8b60e25ed1b
# spawn 时用:A2 构建完把 module ID 填这里;VMDOCKER_SCHEDULER 是调度节点地址
VMDOCKER_MODULE_ID=
VMDOCKER_SCHEDULER=
# spawn 用哪个 runtime backend:docker(容器,秒级)或 sandbox(macOS Docker Sandbox VM,慢)。
# examples 把它作为 Runtime-Backend spawn tag 传给节点;留空则在 macOS 上默认走 sandbox。
RUNTIME_BACKEND=docker
# A4 export 用:要 export 的进程 pid(也可作为命令行参数传:go run ./examples export <pid>)
VMDOCKER_EXPORT_PID=
```

谁读哪些:
- **`cmd/module`**:`VMDOCKER_AGENT_BIN`、`VMDOCKER_URL`、`VMDOCKER_PRIVATE_KEY`
- **`examples`**:`VMDOCKER_URL`、`VMDOCKER_PRIVATE_KEY`、`VMDOCKER_MODULE_ID`、`VMDOCKER_SCHEDULER`、
  `RUNTIME_BACKEND`、`VMDOCKER_EXPORT_PID`

两者读的是**同一个**根 `.env`(`examples` 由 `examples/env.go` 支持,`cmd/module` 也已支持)。

---

## profile 完整配置

建一个自包含的 agent 目录。`[dockerfile].bin` 和 `[dockerfile].startup` 是**必填**(生成器强制);
`FROM` 填**完整镜像名**,原样作为 Dockerfile 的 FROM(不再做别名映射)。RUNTIME_TYPE 不在 profile 里
—— 它是 spawn 时的 `Container-Env-RUNTIME_TYPE` tag(examples 从 `.env` 的 `RUNTIME_TYPE` 读)。

```
myagent/
├── profile.toml
├── bin/                 # 你的可执行文件;整个目录 COPY 到 /usr/local/bin(并 +x)
│   └── .keep            # 即使没有可执行文件也留个 .keep 保住目录
├── start.sh             # 运行时启动钩子(不是容器 ENTRYPOINT)
├── skills/              # public 内容(build 从这里采集打进 module,spawn 时种入;Export 从活 workspace 采集)
│   └── soul.md
├── persona/
│   └── style.md
└── investment.md
```

> **`bin/` 放什么(重要,别搞混):** `bin/` 只放**你自己的可执行程序**(agent 要调用的工具),
> **可以为空**。**adapter 二进制(`vmdocker-agent`)不放这里** —— 它由构建时的 `--agent-bin`
> (即 `VMDOCKER_AGENT_BIN`,见第 0 步 / A2)自动注入成镜像 ENTRYPOINT。生成的 Dockerfile 分两条
> 独立 COPY:一条把 `--agent-bin` 的 adapter 放到 `/usr/local/bin/vmdocker-agent`,另一条把你的
> `bin/` 整目录放到 `/usr/local/bin/`。所以 `bin/` 只留个 `.keep` 空着即可,不需要往里拷任何东西。

### `myagent/profile.toml`

```toml
# vmdockerv2 agent module 的声明式配方。
#   [dockerfile] -> 喂给标准化 Dockerfile 生成器
#   [vmdocker]   -> 喂给运行时 Export/Import 的 public 白名单
# 两段互不串用。

[dockerfile]
# 完整基础镜像名,原样作为 Dockerfile 的 FROM(不做别名映射)。
# RUNTIME_TYPE 不在这里 —— spawn 时用 Container-Env-RUNTIME_TYPE tag 传(examples 从 .env RUNTIME_TYPE 读),
# 决定 adapter 健康就绪:claude=等 claude 在 PATH、openclaw=等网关、空/test=永远就绪。
FROM = "docker/sandbox-templates:claude-code"

# 你的可执行文件目录。整目录 COPY 进 /usr/local/bin 并 chmod +x。
# 必填 —— 可以为空(留个 .keep 保证目录存在)。
bin = "bin"

# 你的启动钩子。adapter(PID 1)在后台跑它;它不是容器 ENTRYPOINT,也不能阻塞 adapter 启动。必填。
startup = "start.sh"

# 要安装的跨发行版工具包(可选;展开为一条包管理器 RUN)。
tools = ["ripgrep", "jq"]

# 额外的 Dockerfile RUN 行 —— 每个值**不含**开头的 "RUN "(可选)。
RUN = ["echo built-from-profile > /home/hymx/.build-marker"]

[vmdocker]
# HOME 相对的 public 白名单。build 时 cmd/module 从 profile 目录采集、Export 时从活 workspace 采集,
# 两者都打进 public.zip,spawn 时叠加进全新 workspace。
#   "~/目录/*" = 目录(递归);  "~/文件" = 单个文件。
# HOME 里没列进来的一切都是私有的,永不导出。
public = ["~/skills/*", "~/persona/*", "~/investment.md"]
```

### `myagent/start.sh`

对 **claude**,就绪条件只是 `claude` 在 `PATH` 上,所以钩子可以是空操作:

```sh
#!/bin/sh
# 作者钩子:在这里起引擎 / 种入状态,然后返回(引擎继续在后台跑)。
# claude 的就绪 = 基础镜像自带的 `claude` CLI 在 PATH 上,所以空操作即可。
exit 0
```

对 **openclaw**,在这里启动网关(adapter 用它来判 `/vmm/health` 就绪)—— 具体网关命令看 openclaw
基础镜像。

### 一键创建

```bash
mkdir -p /tmp/myagent/bin /tmp/myagent/skills /tmp/myagent/persona
cat > /tmp/myagent/profile.toml <<'TOML'
[dockerfile]
FROM = "docker/sandbox-templates:claude-code"
bin = "bin"
startup = "start.sh"
tools = ["ripgrep", "jq"]
RUN = ["echo built-from-profile > /home/hymx/.build-marker"]

[vmdocker]
public = ["~/skills/*", "~/persona/*", "~/investment.md"]
TOML
printf 'keep\n'              > /tmp/myagent/bin/.keep
printf '#!/bin/sh\nexit 0\n' > /tmp/myagent/start.sh ; chmod +x /tmp/myagent/start.sh
printf 'MY-SOUL\n'           > /tmp/myagent/skills/soul.md
printf 'terse, precise\n'    > /tmp/myagent/persona/style.md
printf 'thesis: X\n'         > /tmp/myagent/investment.md
```

---

## 路线 A —— 完整节点

### A0. 基础设施

```bash
redis-server &                                  # 或:docker run -d -p 6379:6379 redis
# cmd/config.yaml 里 arweaveURL 指向 https://arweave.net,redis 指向 localhost:6379。
```

### A1. 起节点(vmdockerv2)

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
go run ./cmd --config cmd/config.yaml           # 或:./build/hymx-node --config cmd/config.yaml
```

配置要点(`cmd/config.yaml`):`port: :8080`、`redisURL`、`arweaveURL`、节点 `prvKey` +
`keyfilePath`。首次运行可能还要做节点引导 —— 另开一个终端:

```bash
export VMDOCKER_URL=http://127.0.0.1:8080
export VMDOCKER_PRIVATE_KEY=0x64dd2342616f385f3e8157cf7246cf394217e13e8f91b7d208e9f8b60e25ed1b
go run ./examples init                          # init token + registry(见 examples/init.go)
# 若你的配置要求注册/质押,给节点密钥转账并质押
#   (见 examples/main.go 的 `transfer` 和 examples/hm.go 的 `stake`)
```

### A2. 构建镜像 + module

用 **vmdockerv2 自己的** `cmd/module`。**不要**用 `go run ./examples module` —— 那个会去跑
`vmdocker_agent/cmd/module`,而 entrypoint-only 分支已经把它删了,会失败。

```bash
export VMDOCKER_URL=http://127.0.0.1:8080
export VMDOCKER_PRIVATE_KEY=0x64dd2342616f385f3e8157cf7246cf394217e13e8f91b7d208e9f8b60e25ed1b
go run ./cmd/module --profile /tmp/myagent/profile.toml --agent-bin "$VMDOCKER_AGENT_BIN"
#   真 docker build -> docker save -> 打包 + 签名 -> 上传到节点
#   打印:  [module] saved module <ID> -> mod-<ID>.json
```

### A3. spawn 这个 module

```bash
export VMDOCKER_MODULE_ID=<A2 得到的 ID>
export VMDOCKER_SCHEDULER=<config 里 prvKey 对应的节点地址>
go run ./examples spawn                          # s.Spawn(module, scheduler, tags)
#   打印 spawn 出的进程 pid
```

### A4. export 一个运行中的进程

现在有现成的 example(`examples/export.go`)。export 是一条 `Apply(Action=Export)` 消息:

```bash
go run ./examples export <A3 的 pid>       # 或在 .env 里设 VMDOCKER_EXPORT_PID 后直接 go run ./examples export
#   加 VMDOCKER_EXPORT_DRY_RUN=1 只预览 public 清单、不产出 module
#   成功后打印:  exported module id: <ID2>
#                the node wrote mod-<ID2>.json into its module store
```

它做的事:发 `Action=Export` 消息 → **节点复用进程正在跑的镜像**(不重建)+ 当前 `profile.toml` +
现采的 `public.zip` 打包成新 module → **节点把它写进自己的 module 目录**(`mod/mod-<ID2>.json`)→
结果只返回 module id。client 读到 id 即可。

> **Export 复用镜像,不重建。** 程序(镜像,含 `bin/`→`/usr/local/bin`、`start.sh`、tools、RUN 结果)
> 原样保留;只有 public 状态(skills/persona…)在 export 时从活 workspace 重新采集。所以**节点侧不需要
> `VMDOCKER_AGENT_BIN`**(可选 `VMDOCKER_MODULE_SIGNER_KEY` 指定签名 key)。

> **为什么返回 id 而不是 module 字节。** module 内嵌完整容器镜像(GB 级)。早期版本把它当结果 `Data`
> 返回,base64 后 ~1GB,超过 redis `proto-max-bulk-len`(512MB),节点报
> `save result failed: ... connection reset by peer`。现在节点直接落盘 + 返回 id,镜像不过 redis。

> **返回值解码(容易踩):** `SendMessageAndWait` 返回的是 `serverSchema.Response{Id, Message}`,
> **没有 `Data` 字段**。module id 在 `res.Message` 里 —— 它是被 JSON 序列化的 `vmmSchema.VmmResult`,
> 解出来读 `.Data`(现在是 id 字符串,不再是 base64 module;错误读 `.Error`)。(路线 B 里
> `vm.Apply(...)` 直接返回 `vmmSchema.Result`,那里读 `res.Data` / `res.Error` 才是对的。)

### A5. spawn 导出的 module

A4 已经把 module 写到了 `cmd/mod/mod-<ID2>.json`,节点启动时从这里加载。所以:

```bash
# 在 .env 里把 VMDOCKER_MODULE_ID 改成 A4 打印的 <ID2>,重启节点让它加载新 module,然后:
go run ./examples spawn
```

(节点是在启动时扫描 `cmd/mod/`,新 module 落盘后需要重启节点才会被加载。)

---

## 路线 B —— 进程内(免节点/Redis/Arweave/质押)

同一套能力代码,直连驱动。这个测试**目前还不在仓库里** —— 下面的步骤定义了一个带 build tag 的测试
(`TestBuildSpawnExportRespawn`,tag `e2e_realspawn`),它在现有的 `vmdocker/realspawn_e2e_test.go`
基础上补上 export + 再 spawn:

1. 用 profile 素材构建 module(`modulebuild.BuildModuleArtifact` + `capability.SignModuleArtifact`),
   写到 `mod/mod-<id>.json`。
2. `vm1, _ := vmdocker.Spawn(env1)` —— 真 docker build 已完成;真容器起来,`/vmm/health` → 200。
3. **Export:** `res := vm1.Apply("tester", vmmSchema.Meta{Action: "Export"})` —— 节点复用 vm1 的镜像,
   把新 module 写进 `mod/mod-<id2>.json`,`res.Data` 返回新 module 的 **id**(不再是 base64 字节)。
4. **再 spawn:** 构造 `env2`(`Process.Module = id2`),`vm2, _ := vmdocker.Spawn(env2)`。
5. 断言两个 spawn 出来的 workspace 都带着 public 内容(宿主侧读
   `sandbox_workspace/<pid>/skills/soul.md`,与 backend 无关)。

运行:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
VMDOCKER_AGENT_BIN=/Users/webbergao/work/src/HymxWorkspace/vmdocker_agent/build/vmdocker-agent \
  go test -tags e2e_realspawn ./vmdocker/ -run TestBuildSpawnExportRespawn -v -count=1
```

---

## 坑位提醒

- **`FROM` 是完整镜像名(原样用),不再有别名。** `RUNTIME_TYPE` 由 spawn 的
  `Container-Env-RUNTIME_TYPE` tag 提供(`.env` 的 `RUNTIME_TYPE` → examples 传);不传则 adapter 默认
  `test`(健康永远就绪)。要 claude 的"等 claude 在 PATH"门控,`.env` 里设 `RUNTIME_TYPE=claude`。
- **`bin` 和 `startup` 必填。** 缺任一个,`GenerateDockerfile` 直接报错。
- **用 vmdockerv2 的 `cmd/module`**,别用 `examples module`(后者指向已删除的
  `vmdocker_agent/cmd/module`)。
- **backend 二选一(`RUNTIME_BACKEND`)。** 留空时 macOS 默认走 **sandbox**(`docker sandbox create`,
  起 VM,~1 分钟;沙箱名会被截断,交互要用 `docker sandbox exec`)。设 `RUNTIME_BACKEND=docker` 走
  **docker container**(`docker run`,秒级,容器名是完整 pid,`docker exec`/`docker logs` 直接可用)——
  **推荐**。断言一律改成宿主侧读 bind-mount 的 workspace(与 backend 无关)。
- **docker backend 的只读 rootfs + 可写 `/tmp`。** docker backend 用 `--read-only` 起容器,adapter 需要
  可写 `/tmp`(start.sh 日志等),否则一启动就崩、容器 Exit(0)。节点已把宿主目录
  `sandbox_workspace/<pid>-tmp` bind 到容器 `/tmp`,所以 `/tmp` 内容在宿主侧可查。
- **`SendMessageAndWait` 没有 `res.Data`。** 它返回 `Response{Id, Message}`,结果在 `res.Message`
  (被序列化的 `vmmSchema.VmmResult`)里,解出来读 `.Data`(export 时是 module id)/ `.Error`。只有
  路线 B 的 `vm.Apply(...)` 直接返回 `vmmSchema.Result`,那里 `res.Data` 才成立。
- **Export 复用现有镜像,不重建**,所以**不需要** `VMDOCKER_AGENT_BIN`;程序(镜像,含 `bin/`)原样
  保留,只重新采集 public 状态。因此 export-after-spawn 是通的(早期"重建镜像"设计因 workspace 无
  `bin/` 而报 `stage bin: ...`,已随复用镜像修复)。
- **Export 返回 module id,不返回字节。** 节点把 module 落盘到 `mod/mod-<id>.json` 再返回 id;镜像是
  GB 级,直接当结果返回会撑爆 redis `proto-max-bulk-len`(512MB)。
- 编 adapter 需要 `GOPRIVATE` + GitHub SSH(公共 proxy 对 `claude-gw` 返回 404)。
