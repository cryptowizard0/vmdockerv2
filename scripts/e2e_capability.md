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

Path-safety negatives (`..`/absolute/symlink escapes, oversize) are covered by
the Go unit tests in `vmdocker/capability/zip_test.go`, not by this script.

## Run

    bash scripts/e2e_capability.sh

Env knobs: `BASE_IMAGE` (default `alpine:3.20`), `CONTAINER_NAME`,
`CLEANUP_ON_EXIT` (default `true`).
