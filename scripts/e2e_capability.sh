#!/usr/bin/env bash
# End-to-end test for the capability seed + import hardening (commit 9e5f2c3).
#
# Part A (always runs, no docker): hardening negative cases driven through the
#   real capability code via the vmme2e CLI — whitelist, size cap, no-profile-
#   overwrite, and overwrite-rollback-restore.
# Part B (needs docker): drives a REAL container with the per-pid workspace bind-
#   mounted at /home/hymx and asserts that host-side seed + import are reflected
#   inside the container (the P3 mount contract + fix #1/#2), via docker exec.
#
# Export/Import are host-side (intercepted in vmdocker.Apply), so they are driven
# by the vmme2e shim, not by curling the container's /vmm.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

BASE_IMAGE="${BASE_IMAGE:-alpine:3.20}"
CONTAINER_NAME="${CONTAINER_NAME:-vmdocker-e2e-capability}"
CLEANUP_ON_EXIT="${CLEANUP_ON_EXIT:-true}"

# NB: use a clean temp path (no template) — a trailing-slash TMPDIR would yield a
# double slash that trips applyPublicZip's raw-string HOME-prefix check.
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

ok()   { echo "[OK] $*"; PASS=$((PASS + 1)); }
bad()  { echo "[FAIL] $*"; FAIL=$((FAIL + 1)); }

# run the CLI, capturing exit code without tripping set -e
run() { set +e; "$@"; local rc=$?; set -e; return "$rc"; }

profile_toml() { # $1=public-list  e.g. '"skills/", "note.md"'
  printf '[dockerfile]\nFROM="openclaw"\nbin="bin"\nstartup="s.sh"\n[vmdocker]\npublic=[%s]\n' "$1"
}

echo "== building vmme2e =="
go build -o "${VMME2E}" ./cmd/vmme2e/

###############################################################################
echo "== Part A: hardening negative cases (no docker) =="
###############################################################################
WSB="${WORKDIR}/wsB"
mkdir -p "${WSB}/skills"
printf 'orig' > "${WSB}/skills/a.md"
profile_toml '"skills/"' > "${WSB}/profile.toml"

# A1: out-of-whitelist path rejected, nothing written
mal="${WORKDIR}/mal"; mkdir -p "${mal}/secret"; printf 'x' > "${mal}/secret/leak"
"${VMME2E}" pack-synthetic --profile "${WSB}/profile.toml" --public-dir "${mal}" --out "${WORKDIR}/mal.json" >/dev/null
if run "${VMME2E}" import --workspace "${WSB}" --module-file "${WORKDIR}/mal.json" 2>"${WORKDIR}/a1.err"; then
  bad "A1 whitelist: import should have failed"
elif grep -q UNAUTHORIZED_PATH "${WORKDIR}/a1.err" && [ ! -e "${WSB}/secret/leak" ]; then
  ok "A1 whitelist: UNAUTHORIZED_PATH, no out-of-whitelist file written"
else
  bad "A1 whitelist: wrong error or leak written ($(cat "${WORKDIR}/a1.err"))"
fi

# A2: happy path adds a whitelisted file
okdir="${WORKDIR}/ok"; mkdir -p "${okdir}/skills"; printf 'NEW' > "${okdir}/skills/new.md"
"${VMME2E}" pack-synthetic --profile "${WSB}/profile.toml" --public-dir "${okdir}" --out "${WORKDIR}/ok.json" >/dev/null
if run "${VMME2E}" import --workspace "${WSB}" --module-file "${WORKDIR}/ok.json" >"${WORKDIR}/a2.out" 2>&1 \
   && [ "$(cat "${WSB}/skills/new.md" 2>/dev/null)" = "NEW" ]; then
  ok "A2 happy path: whitelisted file imported"
else
  bad "A2 happy path: file not imported [out: $(cat "${WORKDIR}/a2.out" 2>/dev/null)] [new.md: $(cat "${WSB}/skills/new.md" 2>/dev/null)]"
fi

# A3: size cap
if run "${VMME2E}" import --workspace "${WSB}" --module-file "${WORKDIR}/ok.json" --max-bytes 10 2>"${WORKDIR}/a3.err"; then
  bad "A3 size cap: should have failed"
elif grep -q TOO_LARGE "${WORKDIR}/a3.err"; then
  ok "A3 size cap: TOO_LARGE"
else
  bad "A3 size cap: wrong error ($(cat "${WORKDIR}/a3.err"))"
fi

# A4: overwrite rollback restores the original (fix #4)
rb="${WORKDIR}/rb"; mkdir -p "${rb}/skills"
printf 'OVERWRITTEN' > "${rb}/skills/a.md"
head -c 2000000 /dev/zero > "${rb}/skills/z.bin"   # compressible: small zip, large expansion
"${VMME2E}" pack-synthetic --profile "${WSB}/profile.toml" --public-dir "${rb}" --out "${WORKDIR}/rb.json" >/dev/null
run "${VMME2E}" import --workspace "${WSB}" --module-file "${WORKDIR}/rb.json" --on-conflict overwrite --max-bytes 100000 >/dev/null 2>&1 || true
if [ "$(cat "${WSB}/skills/a.md")" = "orig" ] && [ ! -e "${WSB}/skills/z.bin" ]; then
  ok "A4 rollback: overwritten file restored to original, partial cleaned up"
else
  bad "A4 rollback: a.md=[$(cat "${WSB}/skills/a.md")] z.bin_present=$(test -e "${WSB}/skills/z.bin" && echo yes || echo no)"
fi

# A5: import never overwrites the target's own profile.toml (fix #2)
before="$(cat "${WSB}/profile.toml")"
wider="${WORKDIR}/wider"; mkdir -p "${wider}/skills"; printf 'z' > "${wider}/skills/w.md"
# module carries a WIDER profile.toml; import must ignore it and keep target's
profile_toml '"skills/", "everything/"' > "${WORKDIR}/wider-profile.toml"
"${VMME2E}" pack-synthetic --profile "${WORKDIR}/wider-profile.toml" --public-dir "${wider}" --out "${WORKDIR}/wider.json" >/dev/null
run "${VMME2E}" import --workspace "${WSB}" --module-file "${WORKDIR}/wider.json" >/dev/null 2>&1 || true
if [ "$(cat "${WSB}/profile.toml")" = "${before}" ]; then
  ok "A5 no-escalation: target profile.toml unchanged after import"
else
  bad "A5 no-escalation: target profile.toml was modified by import"
fi

###############################################################################
echo "== Part B: real-container reflection (needs docker) =="
###############################################################################
if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "[SKIP] docker unavailable — Part B skipped (Part A already covered the hardening logic)"
else
  if ! docker image inspect "${BASE_IMAGE}" >/dev/null 2>&1 && ! docker pull "${BASE_IMAGE}" >/dev/null 2>&1; then
    echo "[SKIP] base image ${BASE_IMAGE} unavailable — Part B skipped"
  else
    WSC="${WORKDIR}/wsC"
    mkdir -p "${WSC}/skills"
    profile_toml '"skills/"' > "${WSC}/profile.toml"
    printf '# e2e-marker seeded-recipe\n' >> "${WSC}/profile.toml"   # valid TOML comment marker

    # Real container with the per-pid workspace bind-mounted at /home/hymx (P3 contract).
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    docker run -d --name "${CONTAINER_NAME}" -v "${WSC}:/home/hymx" "${BASE_IMAGE}" sleep 600 >/dev/null

    # B1: host-side content is visible inside the container at /home/hymx.
    if docker exec "${CONTAINER_NAME}" sh -c 'grep -q seeded-recipe /home/hymx/profile.toml'; then
      ok "B1 mount: workspace profile.toml visible at /home/hymx (P3 contract)"
    else
      bad "B1 mount: profile.toml not visible in container"
    fi

    # B2: a host-side import is reflected inside the running container.
    imp="${WORKDIR}/impC"; mkdir -p "${imp}/skills"; printf 'REPLICATED' > "${imp}/skills/soul.md"
    "${VMME2E}" pack-synthetic --profile "${WSC}/profile.toml" --public-dir "${imp}" --out "${WORKDIR}/impC.json" >/dev/null
    run "${VMME2E}" import --workspace "${WSC}" --module-file "${WORKDIR}/impC.json" >/dev/null 2>&1 || true
    if docker exec "${CONTAINER_NAME}" sh -c '[ "$(cat /home/hymx/skills/soul.md)" = REPLICATED ]'; then
      ok "B2 import reflected: imported file visible inside container at /home/hymx/skills"
    else
      bad "B2 import reflected: imported file not visible in container"
    fi
  fi
fi

echo
echo "==== e2e capability: ${PASS} passed, ${FAIL} failed ===="
[ "${FAIL}" -eq 0 ] && echo "ALL PASS" || { echo "FAILURES PRESENT"; exit 1; }
