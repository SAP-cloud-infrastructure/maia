// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

//go:build !builtinassets

package ui

import "net/http"

// Assets exposes the static files from disk for dev mode (no build tag).
var Assets http.FileSystem = http.Dir("web/ui/static")

// MantineUIAssets exposes the mantine-ui subdirectory directly,
// so /assets/foo.js maps to web/ui/static/mantine-ui/assets/foo.js.
var MantineUIAssets http.FileSystem = http.Dir("web/ui/static/mantine-ui")
