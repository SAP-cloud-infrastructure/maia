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

### Changed

- POST /{domain}/auth now accepts Content-Type values per RFC 9110 §8.3.1, so
  case variants such as `Application/X-WWW-Form-Urlencoded; charset=utf-8` are
  recognised as form-encoded.
- POST /{domain}/auth redirects now preserve `?global=true` (and the
  `X-Global-Region` header equivalent) so the dashboard load binds to the same
  Keystone backend that authenticated the handoff.
- POST /{domain}/auth no longer promotes a pre-existing X-Auth-Token cookie
  into the request header before the form body is parsed, so a fresh token in
  the body takes precedence over a stale session cookie.

### Deprecated

- Passing auth tokens via ?x-auth-token= URL query parameter (use POST endpoint or X-Auth-Token header)
