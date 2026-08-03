<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add `GET /api/v1/whoami` endpoint returning caller identity (user, project, domain, roles)
- Add `GET /api/v1/projects` endpoint returning caller's scoped project list
- Add modern React UI (Prometheus mantine-ui fork) available at `/ui/` (opt-in via `maia.new_ui_enabled = true`)
- Add `maia.new_ui_enabled` config flag (default `false`) to gate the new UI; expose via Helm as `maia.newUI.enabled`
- Add "Try new UI →" banner in classic UI when `maia.new_ui_enabled = true`
- Add go:embed-based asset embedding for the new UI, replacing go-bindata
- Add sentinel label value for global metric visibility (`maia.label_value_for_global_visibility` config option, disabled by default)

### Security

- Prevent cross-tenant metric access: strip client-supplied `X-Project-Id`/`X-Domain-Id` scope headers on authentication so scope is always derived from the validated token
- Verify project membership before honoring the `project_id` query parameter, so an authenticated user cannot read another tenant's metrics by supplying a foreign project ID
- Bump `github.com/prometheus/prometheus` to v0.311.3 (CVE-2026-42151, CVE-2026-42154, CVE-2026-44903)
- Enforce upstream host equality on Prometheus storage requests as defense-in-depth against SSRF (CodeQL `go/request-forgery`)
