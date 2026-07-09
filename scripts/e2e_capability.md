# e2e_capability.sh

End-to-end test for spawn-time clone seeding.

## What it covers

- **Part A (no docker):** packs a synthetic module (stub image + profile + a
  `public.zip`) with `vmme2e pack-synthetic`, then runs the real spawn seed via
  `vmme2e seed-clone` (`vmdocker.SeedWorkspaceFromModule` →
  `capability.UnpackPublicZip`). Asserts both `profile.toml` and the public
  content land in a fresh workspace.
- **Part B (needs docker):** bind-mounts the seeded workspace at `/home/hymx` in
  a real container and asserts the seeded public content is visible inside (the
  P3 mount contract).
- **Part C (opt-in, `RUN_REAL_SPAWN=1`):** the heavyweight path — a real
  `docker build` of a module, then a real in-process `vmdocker.Spawn`, asserting
  the spawned container carries the public content. Implemented as the
  build-tagged Go test `vmdocker/realspawn_e2e_test.go`
  (`go test -tags e2e_realspawn ./vmdocker/ -run TestRealBuildSpawn`). It skips
  cleanly unless docker + a pullable `docker/sandbox-templates:claude-code` +
  the adapter binary are all available.

Path-safety negatives (`..`/absolute/symlink escapes, oversize) are covered by
the Go unit tests in `vmdocker/capability/zip_test.go`, not by this script.

## Run

    bash scripts/e2e_capability.sh                 # Part A + Part B
    RUN_REAL_SPAWN=1 bash scripts/e2e_capability.sh # + Part C (real build + spawn)

Env knobs: `BASE_IMAGE` (default `alpine:3.20`), `CONTAINER_NAME`,
`CLEANUP_ON_EXIT` (default `true`), `RUN_REAL_SPAWN` (enable Part C).

Part C extra knobs: `VMDOCKER_AGENT_BIN` (prebuilt linux adapter binary — avoids
building the sibling `vmdocker_agent` repo), `VMDOCKER_AGENT_REPO` (sibling repo
path, default `../../vmdocker_agent`), `VMDOCKER_AGENT_GOARCH` (adapter build
arch, default the host arch).
