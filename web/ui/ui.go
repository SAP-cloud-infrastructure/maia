// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

//go:build !builtinassets

package ui

import "net/http"

// Assets exposes the static files from disk for dev mode (no build tag).
var Assets http.FileSystem = http.Dir("web/ui/static")
