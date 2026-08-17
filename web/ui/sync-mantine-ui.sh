#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0
#
# sync-mantine-ui.sh — assisted re-sync of the vendored Prometheus mantine-ui fork.
#
# This does NOT auto-merge. The Maia patches are interleaved with upstream code,
# so a machine merge is unsafe. This script's job is to make the human merge fast
# and reviewable: it fetches a target upstream tag, then prints the exact upstream
# delta (since our pinned baseline) for the paths we actually fork. You then
# re-apply the Maia patch points (see web/ui/UPSTREAM.md) onto the new upstream.
#
# Usage:
#   web/ui/sync-mantine-ui.sh                 # report drift: pinned baseline -> latest upstream
#   web/ui/sync-mantine-ui.sh v3.13.2         # report upstream diff: pinned baseline -> v3.13.2
#   web/ui/sync-mantine-ui.sh v3.13.2 --keep  # ...and leave the fetched upstream tree on disk
#
# Output: a unified diff of the forked paths between the pinned SHA and the target
# tag, plus a summary of which forked files changed upstream. Read-only w.r.t. the
# working tree — it never modifies web/ui/.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UPSTREAM_MD="${SCRIPT_DIR}/UPSTREAM.md"

# --- parse pinned provenance -------------------------------------------------
if [[ ! -f "${UPSTREAM_MD}" ]]; then
  echo "ERROR: ${UPSTREAM_MD} not found — cannot determine pinned baseline." >&2
  exit 1
fi

# Extract KEY=VALUE lines from the fenced block in UPSTREAM.md.
read_pin() { grep -E "^$1=" "${UPSTREAM_MD}" | head -n1 | cut -d= -f2-; }
UPSTREAM_REPO="$(read_pin UPSTREAM_REPO)"
PINNED_VERSION="$(read_pin UPSTREAM_VERSION)"
PINNED_SHA="$(read_pin UPSTREAM_SHA)"

if [[ -z "${UPSTREAM_REPO}" || -z "${PINNED_SHA}" ]]; then
  echo "ERROR: could not parse UPSTREAM_REPO / UPSTREAM_SHA from ${UPSTREAM_MD}." >&2
  exit 1
fi

TARGET_REF="${1:-}"
KEEP=false
[[ "${2:-}" == "--keep" ]] && KEEP=true

# Paths we fork (relative to the upstream repo root, which mirrors web/ui/).
FORKED_PATHS=(
  "web/ui/mantine-ui"
  "web/ui/module"
  "web/ui/package.json"
  "web/ui/pnpm-workspace.yaml"
  "web/ui/build_ui.sh"
)

echo ">> pinned baseline: ${PINNED_VERSION} (${PINNED_SHA})"
echo ">> upstream repo:   ${UPSTREAM_REPO}"

# --- fetch upstream into a scratch clone -------------------------------------
WORKDIR="$(mktemp -d)"
cleanup() { [[ "${KEEP}" == true ]] || rm -rf "${WORKDIR}"; }
trap cleanup EXIT

echo ">> cloning upstream (blobless, filtered to forked paths)…"
git clone --filter=blob:none --no-checkout --quiet "${UPSTREAM_REPO}" "${WORKDIR}/upstream"
cd "${WORKDIR}/upstream"

# Resolve target: explicit tag arg, or the latest STABLE vX.Y.Z tag if none
# given. Pre-releases (-rc, -beta, -alpha) are excluded from the auto-pick so a
# no-arg run never steers a maintainer onto a release candidate; pass such a tag
# explicitly if you really want it.
if [[ -z "${TARGET_REF}" ]]; then
  git fetch --tags --quiet
  TARGET_REF="$(git tag -l 'v[0-9]*' | grep -Ev '\-(rc|beta|alpha)' | sort -V | tail -n1)"
  echo ">> no target given — latest stable upstream tag is ${TARGET_REF}"
fi

git fetch --quiet origin "${PINNED_SHA}" || true
git fetch --tags --quiet origin "${TARGET_REF}"
TARGET_SHA="$(git rev-parse FETCH_HEAD)"

if [[ "${TARGET_SHA}" == "${PINNED_SHA}" ]]; then
  echo ">> already on the pinned baseline — nothing to sync."
  exit 0
fi

# --- report the upstream delta for forked paths ------------------------------
echo ""
echo "==================================================================="
echo " Upstream drift: ${PINNED_VERSION} (${PINNED_SHA:0:12}) -> ${TARGET_REF} (${TARGET_SHA:0:12})"
echo "==================================================================="
echo ""
echo ">> forked files changed upstream in this range:"
git diff --stat "${PINNED_SHA}" "${TARGET_SHA}" -- "${FORKED_PATHS[@]}" || {
  echo "   (could not diff — one of the refs may be unfetched; try re-running)"
}

DIFF_OUT="${SCRIPT_DIR}/../../build/mantine-ui-upstream-${TARGET_REF}.diff"
mkdir -p "$(dirname "${DIFF_OUT}")"
git diff "${PINNED_SHA}" "${TARGET_SHA}" -- "${FORKED_PATHS[@]}" > "${DIFF_OUT}" || true

echo ""
echo ">> full upstream diff written to: ${DIFF_OUT}"
echo ""
echo "NEXT STEPS (manual — see web/ui/UPSTREAM.md 'Maia patch points'):"
echo "  1. Review ${DIFF_OUT} to understand what changed upstream."
echo "  2. Re-apply the Maia patches onto the new upstream tree."
if [[ "${KEEP}" == true ]]; then
  echo "     Fetched upstream tree kept at: ${WORKDIR}/upstream (checked out at ${TARGET_REF})"
  git checkout --quiet "${TARGET_SHA}" -- "${FORKED_PATHS[@]}" 2>/dev/null || true
fi
echo "  3. Update the pinned block in web/ui/UPSTREAM.md:"
echo "        UPSTREAM_VERSION=${TARGET_REF}"
echo "        UPSTREAM_SHA=${TARGET_SHA}"
echo "        SYNCED_ON=<today>"
echo "  4. make generate && make check"
echo "  5. Run web/ui/../.. osv-scan (or wait for the weekly workflow) to re-check CVEs."
