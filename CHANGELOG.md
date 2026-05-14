<!--
SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- POST /{domain}/auth endpoint for token handoff via request body

### Deprecated

- Passing auth tokens via ?x-auth-token= URL query parameter (use POST endpoint or X-Auth-Token header)
