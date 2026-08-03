#!/bin/bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0
#
# Builds the Maia React UI (mantine-ui fork of Prometheus v3.11.2).
# Output: web/ui/static/mantine-ui/
#
# Usage:
#   bash web/ui/build_ui.sh          # build everything
#   bash web/ui/build_ui.sh --mantine-ui  # build mantine-ui only (skips lezer/codemirror)

set -euo pipefail

# Run non-interactively everywhere. Without a TTY (Concourse/BuildKit), pnpm
# otherwise aborts with ERR_PNPM_ABORTED_REMOVE_MODULES_DIR_NO_TTY when it needs
# to reconcile a stale node_modules. CI=true + confirmModulesPurge=false make
# the install deterministic on a laptop, in Docker, and in the pipeline alike.
export CI=true
export npm_config_confirm_modules_purge=false

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATIC_DIR="${SCRIPT_DIR}/static"

# Bootstrap pnpm if it is not already on PATH. The GitHub Actions runner (and a
# bare `make generate`) only provisions Go, not a JS toolchain, but the Node it
# ships bundles corepack, which can activate the exact pnpm pinned in
# web/ui/package.json ("packageManager"). This keeps the runner, Docker, and a
# laptop all on the same pnpm that wrote pnpm-lock.yaml, so --frozen-lockfile
# below stays deterministic. Docker still pre-installs pnpm for speed; there
# this branch is skipped.
if ! command -v pnpm >/dev/null 2>&1; then
  echo ">> pnpm not found — bootstrapping via corepack"
  corepack enable
  corepack prepare --activate
fi

if ! [[ -w $HOME ]]; then
  export npm_config_cache
  npm_config_cache=$(mktemp -d)
fi

function buildModules() {
  echo ">> building lezer-promql and codemirror-promql"
  pnpm --filter @prometheus-io/lezer-promql run build
  pnpm --filter @prometheus-io/codemirror-promql run build
}

function buildMantineUI() {
  echo ">> building mantine-ui"
  pnpm --filter @prometheus-io/mantine-ui run build
  mkdir -p "${STATIC_DIR}"
  rm -rf "${STATIC_DIR}/mantine-ui"
  mv "${SCRIPT_DIR}/mantine-ui/dist" "${STATIC_DIR}/mantine-ui"
  echo ">> output: web/ui/static/mantine-ui/"
}

# Install dependencies first (frozen lockfile — fails if pnpm-lock.yaml is stale)
echo ">> pnpm install"
pnpm --dir "${SCRIPT_DIR}" install --frozen-lockfile

case "${1:---all}" in
  --mantine-ui)
    buildModules
    buildMantineUI
    ;;
  --all | *)
    buildModules
    buildMantineUI
    ;;
esac
