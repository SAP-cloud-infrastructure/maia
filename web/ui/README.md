<!--
SPDX-FileCopyrightText: The Prometheus Authors
SPDX-License-Identifier: Apache-2.0
-->

# Maia Web UI

This directory contains Maia's React web UI. It is a **hard fork** of the
Prometheus [`mantine-ui`](https://github.com/prometheus/prometheus/tree/main/web/ui)
frontend, copied in at a pinned upstream tag and patched with Maia-specific
changes (Keystone-scoped identity, project switcher, `/ui/` base path, removed
Prometheus-only surfaces, branding).

> **Provenance & how to update:** see [`UPSTREAM.md`](./UPSTREAM.md). It records
> the exact upstream repo, tag, and commit SHA, lists the Maia patch points, and
> documents the tooling-assisted re-sync procedure (`sync-mantine-ui.sh`).
>
> **This is copied source, not a tracked dependency.** `go.mod` and Renovate do
> not track it. Upstream drift is surfaced by a Renovate customManager watching
> the version line in `UPSTREAM.md`; JS-dependency CVEs are surfaced by the
> weekly [`osv-scan`](../../.github/workflows/osv-scan.yaml) workflow (run
> locally with `make check-ui-cves`).

## Layout

* `mantine-ui/` — the React SPA (`@prometheus-io/mantine-ui`), with Maia patches.
* `module/` — shared npm modules for PromQL editing via
  [CodeMirror](https://codemirror.net/): `@prometheus-io/lezer-promql` and
  `@prometheus-io/codemirror-promql`.
* `static/` — build output. Compiled into the Maia binary via Go's `embed`
  package (see `assets_embed.go`). Generated — not checked in.

Maia ships a **single** React UI. Upstream's old 2.x `react-app/` and its
`--enable-feature=old-ui` flag were **not** forked and do not exist here.

## Prerequisites

* Node `v22.21.1` (see `.nvmrc`)
* pnpm `10.33.0` — bootstrapped automatically via corepack by `build_ui.sh`

The workspace uses **pnpm**, not npm. `pnpm-lock.yaml` is authoritative and the
build uses `--frozen-lockfile`. Do not run `npm install` here.

## Building

The UI is built as part of the repo's code generation, not with a separate
`ui-build` target:

```bash
make generate      # from the repo root — builds the UI into web/ui/static/
```

Under the hood this runs `web/ui/build_ui.sh` (pnpm install + build of the
`mantine-ui` app and the `module/*` packages). To build just the UI:

```bash
cd web/ui && bash build_ui.sh            # everything
cd web/ui && bash build_ui.sh --mantine-ui
cd web/ui && bash build_ui.sh --build-module
```

`make build` (repo root) produces the Maia binary with the built assets
embedded via the `builtinassets` build tag.

## Local development server

You can iterate on the SPA outside a running Maia server with Vite's dev server:

```bash
cd web/ui/mantine-ui && pnpm start       # http://localhost:5173/
```

The page hot-reloads on source edits and shows lint errors in the console.
Hot reload covers `mantine-ui/` only; for changes under `module/` (the
CodeMirror PromQL editor) run `bash build_ui.sh --build-module` from `web/ui`.

The dev server proxies API requests to a backend per `mantine-ui/vite.config.ts`
(`http://localhost:9090` by default). Point a local Maia or Prometheus at that
address, or edit `vite.config.ts` to target another backend (add
`changeOrigin: true` for HTTPS backends).

## Tests

```bash
cd web/ui/mantine-ui && pnpm test        # watch mode
cd web/ui/mantine-ui && CI=true pnpm test  # run once and exit
```

Run a single module's tests from that module's directory.

## Serving assets from the filesystem (advanced)

By default assets are compiled into the Maia binary (`builtinassets` tag). During
development it is usually simpler to use the Vite dev server above. If you do
want the Go binary to serve assets from `web/ui/static/` on disk, build without
the `builtinassets` tag (see `assets_embed.go` and the `//go:build` tags there),
and rebuild the assets with `bash build_ui.sh` first.
