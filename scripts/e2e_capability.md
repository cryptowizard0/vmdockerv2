# e2e_capability.sh — 能力 Export/Import 端到端测试

验证 capability 的 **seed profile + import 硬化**（提交 `9e5f2c3`）在贴近真实运行时的形态下端到端可用，并守住安全不变量。

## 为什么需要这个脚本

Export/Import 是 **host 侧**逻辑——在 `vmdocker.Apply`（`applyCapabilityAction`）里被拦截处理，**不经过容器的 `/vmm`**。所以：

- 纯 `curl` 容器无法触发 Export/Import；
- 因此用一个很薄的 Go CLI `cmd/vmme2e` 驱动真实的 capability 代码（无新逻辑），bash 负责编排容器 + 断言。

## 组成

| 文件 | 作用 |
|---|---|
| `scripts/e2e_capability.sh` | 测试编排脚本（bash） |
| `cmd/vmme2e/main.go` | 薄 CLI：`seed` / `export` / `import` / `pack-synthetic`，包住真实代码 |
| `vmdocker/module_image.go` → `SeedWorkspaceProfileFromModule` | 导出的 seed 包装器，供 CLI 调真实的 spawn-time seed |

## 测试内容

脚本分两部分：

### Part A：硬化负例（无需 docker，随处可跑）

用 `vmme2e pack-synthetic` 造模块 + `vmme2e import` 打进临时 workspace，逐条校验不变量：

| 用例 | 断言 | 对应修复 |
|---|---|---|
| A1 白名单 | `public.zip` 含越界路径 `secret/leak` → `UNAUTHORIZED_PATH`，不落盘 | 写入锁定目标白名单 |
| A2 正常复刻 | 白名单内文件被导入 | round-trip |
| A3 大小上限 | `--max-bytes 10` → `TOO_LARGE` | #3 size cap |
| A4 回滚恢复 | overwrite 中途失败 → 原文件**恢复**、半成品清理 | #4 rollback restore |
| A5 不提权 | 模块带**更宽**的 `profile.toml` → 目标 `profile.toml` 不变 | #2 不覆盖 profile |

### Part B：真容器反映（需要 docker）

把 per-pid workspace **bind-mount 到真实容器的 `/home/hymx`**（P3 契约），用 `docker exec` 断言 host 侧的 seed / import 在容器内可见：

| 用例 | 断言 |
|---|---|
| B1 挂载 | workspace 的 `profile.toml` 在容器内 `/home/hymx` 可见（含唯一 marker） |
| B2 导入反映 | host 侧一次 import 后，容器内 `/home/hymx/skills/soul.md` 出现 |

> Part B 用广泛可得的 `alpine`（`sleep`）容器，只为验证「挂载 + host 侧读写反映进容器」这条链路；容器内不跑 adapter。若 docker 或基础镜像不可用，Part B **自动跳过**，Part A 已覆盖硬化逻辑。

## 前置条件

- Go 工具链（脚本会 `go build ./cmd/vmme2e`）。
- Part A：无其它依赖。
- Part B：可用的 `docker`（`docker info` 通过）+ 可拉取/本地存在的 `BASE_IMAGE`。

## 使用

```bash
cd /path/to/vmdocker
./scripts/e2e_capability.sh
```

环境变量（均可选）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `BASE_IMAGE` | `alpine:3.20` | Part B 用的容器镜像 |
| `CONTAINER_NAME` | `vmdocker-e2e-capability` | 测试容器名 |
| `CLEANUP_ON_EXIT` | `true` | 退出时 `docker rm -f` + 删临时目录；设 `false` 便于排查 |

退出码：全部通过 `0`；任一断言失败 `1`（并打印 `FAILURES PRESENT`）。`trap cleanup EXIT` 负责清理。

### 预期输出

```
== Part A: hardening negative cases (no docker) ==
[OK] A1 whitelist: UNAUTHORIZED_PATH, no out-of-whitelist file written
[OK] A2 happy path: whitelisted file imported
[OK] A3 size cap: TOO_LARGE
[OK] A4 rollback: overwritten file restored to original, partial cleaned up
[OK] A5 no-escalation: target profile.toml unchanged after import
== Part B: real-container reflection (needs docker) ==
[OK] B1 mount: workspace profile.toml visible at /home/hymx (P3 contract)
[OK] B2 import reflected: imported file visible inside container at /home/hymx/skills
==== e2e capability: 7 passed, 0 failed ====
ALL PASS
```

（无 docker 时 Part B 显示 `[SKIP]`，总计 `5 passed`。）

## 脚本流程

```
build vmme2e
└─ Part A（无 docker）
   造 target workspace（profile.public=["skills/"], skills/a.md=orig）
   A1..A5：pack-synthetic 造模块 → vmme2e import → 校验不变量
└─ Part B（有 docker 才跑）
   docker run -v <WS>:/home/hymx alpine sleep
   B1：docker exec grep marker /home/hymx/profile.toml
   B2：host 侧 vmme2e import → docker exec 校验文件出现在 /home/hymx
汇总 pass/fail → 退出码
```

## vmme2e CLI 参考

薄 CLI，包住真实 capability/modulebuild 代码；命中 `capability.CodedError` 时以退出码 `3` + `CODE: msg`（stderr）返回，便于 bash 断言错误码。

```
vmme2e seed           --module-id <id> --workspace <dir> [--archive-format container-tar+image.tar.gz]
vmme2e export         --workspace <dir> --out <cap.json> [--agent-bin <p> --wrapper <p> --build-tag <t> --signer-key <hex>]
vmme2e import         --workspace <dir> --module-file <cap.json> [--on-conflict skip|overwrite|fail] [--max-bytes N] [--max-module-bytes N]
vmme2e pack-synthetic --profile <p.toml> --public-dir <dir> --out <cap.json> [--image-name N --image-id ID --signer-key hex]
```

- `seed`：`mod-<id>.json` 需能在 CWD 相对路径解析到（`mod/mod-<id>.json` 或 `mod-<id>.json`），同运行时 spawn 路径。
- `export`：真实 Export——会 `docker build` 重建镜像并打包 `public.zip`（重，需 docker + base + adapter）。脚本 Part B 未用它，留作全链路手动验证。
- `pack-synthetic`：不经 docker 直接造 V2 模块（桩 `image.tar.gz` + profile + 把 `--public-dir` **原样**打成 `public.zip`），可造任意/越界路径与可压缩大文件用于负例。

## 故障排查

- **Part B 一直 SKIP**：`docker info` 不通或 `BASE_IMAGE` 拉不到；只想跑硬化逻辑时可忽略。
- **想看容器现场**：`CLEANUP_ON_EXIT=false ./scripts/e2e_capability.sh`，随后 `docker exec -it $CONTAINER_NAME sh`。
- **CI**：无 docker 环境跑 Part A（5 项）即可覆盖硬化不变量；有 docker 的环境自动带上 Part B。
```
