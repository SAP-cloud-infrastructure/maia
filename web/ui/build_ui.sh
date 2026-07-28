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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATIC_DIR="${SCRIPT_DIR}/static"

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
