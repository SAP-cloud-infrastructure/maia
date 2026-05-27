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

- Add sentinel label value for global metric visibility (`maia.label_value_for_global_visibility` config option, disabled by default)

### Security

- Bump `github.com/prometheus/prometheus` to v0.311.3 (CVE-2026-42151, CVE-2026-42154, CVE-2026-44903)
