<!--
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company

SPDX-License-Identifier: Apache-2.0
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- CHANGELOG.md following Keep a Changelog format
- CLAUDE.md for AI assistant guidance

### Changed

- Documentation improvements across README, operators guide, developers guide, users guide, and metrics docs

### Fixed

- Operators guide: global keystone config section name corrected from `[global_keystone]` to `[keystone.global]`
- Operators guide: config example format corrected from YAML to TOML
- Developers guide: replaced stale Travis CI references with GitHub Actions
- Developers guide: replaced stale glide/vendor references with Go modules
- Users guide: fixed GitHub URLs from `SAP-cloud-infrastructure` to `sapcc`
- Users guide: fixed heading level for Federation section (H1 to H2)
- Users guide: added missing sections to table of contents
- Metrics docs: fixed Summary sub-metric types, added labels and descriptions
