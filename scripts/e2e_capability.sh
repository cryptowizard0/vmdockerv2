#!/usr/bin/env bash
# End-to-end test for spawn-time clone seeding (profile.toml + public.zip).
#
# Part A (always runs, no docker): pack a synthetic module carrying public
#   content, run the REAL spawn seed via the vmme2e shim (SeedWorkspaceFromModule
#   -> capability.UnpackPublicZip), and assert profile + public land in a fresh
#   workspace. Path-safety negatives (escape/symlink/oversize) are covered by the
#   Go unit tests in vmdocker/capability/zip_test.go.
# Part B (needs docker): bind-mount the seeded workspace at /home/hymx and assert
#   the seeded public content is visible inside a real container (P3 mount).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

BASE_IMAGE="${BASE_IMAGE:-alpine:3.20}"
CONTAINER_NAME="${CONTAINER_NAME:-vmdocker-e2e-capability}"
CLEANUP_ON_EXIT="${CLEANUP_ON_EXIT:-true}"

WORKDIR="$(cd "$(mktemp -d)" && pwd -P)"
VMME2E="${WORKDIR}/vmme2e"
PASS=0
FAIL=0

cleanup() {
  if [ "${CLEANUP_ON_EXIT}" = "true" ]; then
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    rm -rf "${WORKDIR}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

ok()  { echo "[OK] $*"; PASS=$((PASS + 1)); }
bad() { echo "[FAIL] $*"; FAIL=$((FAIL + 1)); }

echo "== building vmme2e =="
go build -o "${VMME2E}" ./cmd/vmme2e/

###############################################################################
echo "== Part A: clone seed materializes profile + public (no docker) =="
###############################################################################
RUNDIR="${WORKDIR}/run"; mkdir -p "${RUNDIR}/mod"
pubdir="${WORKDIR}/pub"; mkdir -p "${pubdir}/skills"; printf 'SOUL' > "${pubdir}/skills/soul.md"
printf '[dockerfile]\nFROM="openclaw"\n[vmdocker]\npublic=["skills/"]\n' > "${WORKDIR}/clone-profile.toml"
"${VMME2E}" pack-synthetic --profile "${WORKDIR}/clone-profile.toml" --public-dir "${pubdir}" --out "${RUNDIR}/mod/mod-clone.json" >/dev/null

WSD="${WORKDIR}/wsD"
( cd "${RUNDIR}" && "${VMME2E}" seed-clone --module-id clone --workspace "${WSD}" >/dev/null )
if [ "$(cat "${WSD}/skills/soul.md" 2>/dev/null)" = SOUL ] && grep -q 'FROM="openclaw"' "${WSD}/profile.toml" 2>/dev/null; then
  ok "A1 clone seed: profile.toml + public content materialized into fresh workspace"
else
  bad "A1 clone seed: soul.md=[$(cat "${WSD}/skills/soul.md" 2>/dev/null)] profile=[$(cat "${WSD}/profile.toml" 2>/dev/null)]"
fi

###############################################################################
echo "== Part B: real-container reflection (needs docker) =="
###############################################################################
if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "[SKIP] docker unavailable — Part B skipped (Part A already covered the seed logic)"
else
  if ! docker image inspect "${BASE_IMAGE}" >/dev/null 2>&1 && ! docker pull "${BASE_IMAGE}" >/dev/null 2>&1; then
    echo "[SKIP] base image ${BASE_IMAGE} unavailable — Part B skipped"
  else
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    docker run -d --name "${CONTAINER_NAME}" -v "${WSD}:/home/hymx" "${BASE_IMAGE}" sleep 600 >/dev/null
    if docker exec "${CONTAINER_NAME}" sh -c '[ "$(cat /home/hymx/skills/soul.md)" = SOUL ]'; then
      ok "B1 mount: seeded public content visible at /home/hymx/skills (P3 contract)"
    else
      bad "B1 mount: seeded public content not visible in container"
    fi
  fi
fi

echo
echo "==== e2e capability: ${PASS} passed, ${FAIL} failed ===="
[ "${FAIL}" -eq 0 ] && echo "ALL PASS" || { echo "FAILURES PRESENT"; exit 1; }
